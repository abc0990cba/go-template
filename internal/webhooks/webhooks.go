// Package webhooks manages outgoing webhooks: endpoint CRUD, the delivery
// log, and the event fan-out that matches analytics events against
// subscriptions. The HTTP surface lives in http.go.
package webhooks

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/reliability"
)

// Endpoint subscribes one URL to a set of event names ("*" = all).
// The secret signs every delivery with HMAC-SHA256.
type Endpoint struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	Secret      string    `json:"-"`
	Events      []string  `json:"events"`
	Description string    `json:"description"`
	Active      bool      `json:"active"`
	OrgID       string    `json:"org_id,omitempty"`
	CreatedBy   string    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Delivery is one recorded delivery attempt chain of an event to an
// endpoint. Attempts accumulate across retries.
type Delivery struct {
	ID                 string     `json:"id"`
	EndpointID         string     `json:"endpoint_id"`
	Event              string     `json:"event"`
	Payload            string     `json:"payload"`
	Status             string     `json:"status"`
	Attempts           int        `json:"attempts"`
	LastResponseStatus int        `json:"last_response_status"`
	LastError          string     `json:"last_error"`
	DeliveredAt        *time.Time `json:"delivered_at"`
	CreatedAt          time.Time  `json:"created_at"`
}

// validateWebhookURL accepts only absolute http(s) URLs — the delivery
// client must never be pointed at file:// or internal schemes.
func validateWebhookURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return kernel.Invalid("webhook url must be an absolute http(s) url")
	}
	return nil
}

// Fan is the event-facing surface the realtime sink adapts to.
type Fan interface {
	HandleEvent(ctx context.Context, eventName, userID string)
}

// Settings is the slice of the settings feature the feature flag check
// consumes (webhooks.enabled). Primitive-only so the settings service
// satisfies it without webhooks importing settings.
type Settings interface {
	Bool(ctx context.Context, key string) bool
}

// Store persists subscribed endpoints and their delivery log.
type Store interface {
	InsertWebhookEndpoint(ctx context.Context, endpoint Endpoint) error
	UpdateWebhookEndpoint(ctx context.Context, endpoint Endpoint) error
	DeleteWebhookEndpoint(ctx context.Context, id string) error
	GetWebhookEndpoint(ctx context.Context, id string) (Endpoint, error)
	ListWebhookEndpoints(ctx context.Context) ([]Endpoint, error)
	InsertWebhookDelivery(ctx context.Context, delivery Delivery) error
	UpdateWebhookDelivery(ctx context.Context, delivery Delivery) error
	ListWebhookDeliveries(ctx context.Context, endpointID string, limit, offset int) ([]Delivery, error)
}

// Dispatcher pushes a delivery onto the execution path: the asynq
// queue when Redis is configured, an in-process retry loop otherwise.
type Dispatcher interface {
	// Deliver records the delivery and returns immediately; execution is
	// asynchronous and reported through the deliveries table.
	Deliver(ctx context.Context, endpoint Endpoint, delivery Delivery, payload []byte)
}

// Service manages outgoing webhooks: endpoint CRUD plus the event
// fan-out that matches analytics events against subscriptions.
type Service struct {
	store      Store
	dispatcher Dispatcher
	settings   Settings
	client     *http.Client
	breakers   *reliability.Breakers
	bulkheads  *reliability.Bulkheads
	logger     *slog.Logger
}

// NewService wires the service. Dispatcher may be nil in tests;
// deliverInline then runs synchronously. Breakers and bulkheads may be nil —
// deliveries then run unprotected.
func NewService(store Store, dispatcher Dispatcher, settings Settings, client *http.Client, breakers *reliability.Breakers, bulkheads *reliability.Bulkheads, logger *slog.Logger) *Service {
	return &Service{store: store, dispatcher: dispatcher, settings: settings, client: client, breakers: breakers, bulkheads: bulkheads, logger: logger}
}

// Create adds an endpoint; the secret is generated when omitted.
func (s *Service) Create(ctx context.Context, endpoint Endpoint) (Endpoint, error) {
	if err := validateWebhookURL(endpoint.URL); err != nil {
		return Endpoint{}, err
	}
	endpoint.ID = kernel.NewID()
	if endpoint.Secret == "" {
		endpoint.Secret = kernel.NewID()
	}
	now := time.Now()
	endpoint.CreatedAt, endpoint.UpdatedAt = now, now
	if err := s.store.InsertWebhookEndpoint(ctx, endpoint); err != nil {
		return Endpoint{}, err
	}
	return endpoint, nil
}

// Update rewrites url/events/description/active.
func (s *Service) Update(ctx context.Context, endpoint Endpoint) error {
	if err := validateWebhookURL(endpoint.URL); err != nil {
		return err
	}
	existing, err := s.store.GetWebhookEndpoint(ctx, endpoint.ID)
	if err != nil {
		return err
	}
	existing.URL = endpoint.URL
	existing.Events = endpoint.Events
	existing.Description = endpoint.Description
	existing.Active = endpoint.Active
	existing.UpdatedAt = time.Now()
	return s.store.UpdateWebhookEndpoint(ctx, existing)
}

// Delete removes the endpoint; deliveries cascade.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.store.DeleteWebhookEndpoint(ctx, id)
}

// Get returns one endpoint.
func (s *Service) Get(ctx context.Context, id string) (Endpoint, error) {
	return s.store.GetWebhookEndpoint(ctx, id)
}

// List returns every endpoint.
func (s *Service) List(ctx context.Context) ([]Endpoint, error) {
	return s.store.ListWebhookEndpoints(ctx)
}

// Deliveries pages the delivery log of one endpoint.
func (s *Service) Deliveries(ctx context.Context, endpointID string, limit, offset int) ([]Delivery, error) {
	return s.store.ListWebhookDeliveries(ctx, endpointID, limit, offset)
}

// HandleEvent fans one analytics event out to matching endpoints. The
// webhooks feature flag short-circuits the fan-out.
func (s *Service) HandleEvent(ctx context.Context, eventName, userID string) {
	if s.settings != nil && !s.settings.Bool(ctx, "webhooks.enabled") {
		return
	}
	endpoints, err := s.store.ListWebhookEndpoints(ctx)
	if err != nil {
		s.log("webhook fan-out listing failed", err)
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"event":       eventName,
		"user_id":     userID,
		"occurred_at": time.Now().UTC().Format(time.RFC3339),
	})
	for _, endpoint := range endpoints {
		if !endpoint.Active || !webhookMatches(endpoint.Events, eventName) {
			continue
		}
		delivery := Delivery{
			ID: kernel.NewID(), EndpointID: endpoint.ID, Event: eventName,
			Payload: string(payload), Status: "pending", CreatedAt: time.Now(),
		}
		if err := s.store.InsertWebhookDelivery(ctx, delivery); err != nil {
			s.log("webhook delivery insert failed", err)
			continue
		}
		if s.dispatcher != nil {
			s.dispatcher.Deliver(ctx, endpoint, delivery, payload)
			continue
		}
		s.deliver(ctx, endpoint, delivery, payload)
	}
}

// SendTest fires one synthetic event at a single endpoint (operator check).
func (s *Service) SendTest(ctx context.Context, endpointID string) error {
	endpoint, err := s.store.GetWebhookEndpoint(ctx, endpointID)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"event":       "webhook.test",
		"occurred_at": time.Now().UTC().Format(time.RFC3339),
		"source":      "go-template",
	})
	delivery := Delivery{
		ID: kernel.NewID(), EndpointID: endpoint.ID, Event: "webhook.test",
		Payload: string(payload), Status: "pending", CreatedAt: time.Now(),
	}
	if err := s.store.InsertWebhookDelivery(ctx, delivery); err != nil {
		return err
	}
	if s.dispatcher != nil {
		s.dispatcher.Deliver(ctx, endpoint, delivery, payload)
		return nil
	}
	s.deliver(ctx, endpoint, delivery, payload)
	return nil
}

// webhookMatches supports "*" as the subscribe-to-everything wildcard.
func webhookMatches(subscribed []string, event string) bool {
	for _, pattern := range subscribed {
		if pattern == "*" || pattern == event {
			return true
		}
	}
	return false
}

func (s *Service) log(msg string, err error) {
	if s.logger != nil && err != nil {
		s.logger.Error(msg, "err", err)
	}
}
