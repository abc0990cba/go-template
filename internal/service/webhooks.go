package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/reliability"
	"github.com/your-org/go-template/internal/repository"
)

// validateWebhookURL accepts only absolute http(s) URLs — the delivery
// client must never be pointed at file:// or internal schemes.
func validateWebhookURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", validationError("webhook url must be an absolute http(s) url")
	}
	return raw, nil
}

// WebhookFan is the event-facing surface the realtime sink adapts to.
type WebhookFan interface {
	HandleEvent(ctx context.Context, eventName, userID string)
}

// UseInlineDispatcher swaps the (nil) queue dispatcher for the in-process
// retry loop — the single-process development path.
func (s *WebhooksService) UseInlineDispatcher() {
	s.dispatcher = NewInlineDispatcher(s)
}

// InlineWebhookDispatcher runs deliveries in a background goroutine with
// three attempts and short backoff — the single-process development path.
type InlineWebhookDispatcher struct {
	svc *WebhooksService
}

// NewInlineDispatcher builds the in-process dispatcher.
func NewInlineDispatcher(svc *WebhooksService) *InlineWebhookDispatcher {
	return &InlineWebhookDispatcher{svc: svc}
}

// Deliver implements WebhookDispatcher.
func (d *InlineWebhookDispatcher) Deliver(ctx context.Context, endpoint model.WebhookEndpoint, delivery model.WebhookDelivery, payload []byte) {
	go func() {
		bg := context.Background()
		d.svc.deliver(bg, endpoint, delivery, payload)
		for _, delay := range []time.Duration{2 * time.Second, 8 * time.Second} {
			if delivery.Status == "success" {
				return
			}
			time.Sleep(delay)
			d.svc.deliver(bg, endpoint, delivery, payload)
		}
	}()
}

// DeliverQueued re-runs one recorded delivery from the queue path: the
// worker loads endpoint and delivery by id and posts once.
func (s *WebhooksService) DeliverQueued(ctx context.Context, endpointID, deliveryID string) error {
	endpoint, err := s.store.GetWebhookEndpoint(ctx, endpointID)
	if err != nil {
		return err
	}
	deliveries, err := s.store.ListWebhookDeliveries(ctx, endpointID, 100, 0)
	if err != nil {
		return err
	}
	for _, delivery := range deliveries {
		if delivery.ID != deliveryID {
			continue
		}
		s.deliver(ctx, endpoint, delivery, []byte(delivery.Payload))
		return nil
	}
	return repository.ErrNotFound
}

// WebhooksRepo persists subscribed endpoints and their delivery log.
type WebhooksRepo interface {
	InsertWebhookEndpoint(ctx context.Context, endpoint model.WebhookEndpoint) error
	UpdateWebhookEndpoint(ctx context.Context, endpoint model.WebhookEndpoint) error
	DeleteWebhookEndpoint(ctx context.Context, id string) error
	GetWebhookEndpoint(ctx context.Context, id string) (model.WebhookEndpoint, error)
	ListWebhookEndpoints(ctx context.Context) ([]model.WebhookEndpoint, error)
	InsertWebhookDelivery(ctx context.Context, delivery model.WebhookDelivery) error
	UpdateWebhookDelivery(ctx context.Context, delivery model.WebhookDelivery) error
	ListWebhookDeliveries(ctx context.Context, endpointID string, limit, offset int) ([]model.WebhookDelivery, error)
}

// WebhookDispatcher pushes a delivery onto the execution path: the asynq
// queue when Redis is configured, an in-process retry loop otherwise.
type WebhookDispatcher interface {
	// Deliver records the delivery and returns immediately; execution is
	// asynchronous and reported through the deliveries table.
	Deliver(ctx context.Context, endpoint model.WebhookEndpoint, delivery model.WebhookDelivery, payload []byte)
}

// WebhooksService manages outgoing webhooks: endpoint CRUD plus the event
// fan-out that matches analytics events against subscriptions.
type WebhooksService struct {
	store      WebhooksRepo
	dispatcher WebhookDispatcher
	settings   *SettingsService
	client     *http.Client
	breakers   *reliability.Breakers
	logger     *slog.Logger
}

// NewWebhooksService wires the service. Dispatcher may be nil in tests;
// deliverInline then runs synchronously. Breakers may be nil — deliveries
// then run unprotected.
func NewWebhooksService(store WebhooksRepo, dispatcher WebhookDispatcher, settings *SettingsService, client *http.Client, breakers *reliability.Breakers, logger *slog.Logger) *WebhooksService {
	return &WebhooksService{store: store, dispatcher: dispatcher, settings: settings, client: client, breakers: breakers, logger: logger}
}

// Create adds an endpoint; the secret is generated when omitted.
func (s *WebhooksService) Create(ctx context.Context, endpoint model.WebhookEndpoint) (model.WebhookEndpoint, error) {
	if _, err := validateWebhookURL(endpoint.URL); err != nil {
		return model.WebhookEndpoint{}, err
	}
	if endpoint.Secret == "" {
		endpoint.Secret = newID()
	}
	now := time.Now()
	endpoint.CreatedAt, endpoint.UpdatedAt = now, now
	if err := s.store.InsertWebhookEndpoint(ctx, endpoint); err != nil {
		return model.WebhookEndpoint{}, err
	}
	return endpoint, nil
}

// Update rewrites url/events/description/active.
func (s *WebhooksService) Update(ctx context.Context, endpoint model.WebhookEndpoint) error {
	if _, err := validateWebhookURL(endpoint.URL); err != nil {
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
func (s *WebhooksService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteWebhookEndpoint(ctx, id)
}

// Get returns one endpoint.
func (s *WebhooksService) Get(ctx context.Context, id string) (model.WebhookEndpoint, error) {
	return s.store.GetWebhookEndpoint(ctx, id)
}

// List returns every endpoint.
func (s *WebhooksService) List(ctx context.Context) ([]model.WebhookEndpoint, error) {
	return s.store.ListWebhookEndpoints(ctx)
}

// Deliveries pages the delivery log of one endpoint.
func (s *WebhooksService) Deliveries(ctx context.Context, endpointID string, limit, offset int) ([]model.WebhookDelivery, error) {
	return s.store.ListWebhookDeliveries(ctx, endpointID, limit, offset)
}

// HandleEvent fans one analytics event out to matching endpoints. The
// webhooks feature flag short-circuits the fan-out.
func (s *WebhooksService) HandleEvent(ctx context.Context, eventName, userID string) {
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
		delivery := model.WebhookDelivery{
			ID: newID(), EndpointID: endpoint.ID, Event: eventName,
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
func (s *WebhooksService) SendTest(ctx context.Context, endpointID string) error {
	endpoint, err := s.store.GetWebhookEndpoint(ctx, endpointID)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"event":       "webhook.test",
		"occurred_at": time.Now().UTC().Format(time.RFC3339),
		"source":      "go-template",
	})
	delivery := model.WebhookDelivery{
		ID: newID(), EndpointID: endpoint.ID, Event: "webhook.test",
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

// deliver posts the payload once and records the attempt. It is the body
// of one retry round for both the inline path and the queue worker.
func (s *WebhooksService) deliver(ctx context.Context, endpoint model.WebhookEndpoint, delivery model.WebhookDelivery, payload []byte) {
	if s.settings != nil && !s.settings.Bool(ctx, "webhooks.enabled") {
		return
	}
	body := payload
	if body == nil {
		body = []byte(delivery.Payload)
	}
	mac := hmac.New(sha256.New, []byte(endpoint.Secret))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	status, err := s.post(ctx, endpoint.URL, body, signature, delivery.ID)
	delivery.Attempts++
	delivery.LastResponseStatus = status
	switch {
	case err != nil:
		delivery.Status = "failed"
		delivery.LastError = err.Error()
	case status >= 200 && status < 300:
		delivery.Status = "success"
		now := time.Now()
		delivery.DeliveredAt = &now
	default:
		delivery.Status = "failed"
		delivery.LastError = fmt.Sprintf("endpoint answered %d", status)
	}
	if updateErr := s.store.UpdateWebhookDelivery(ctx, delivery); updateErr != nil {
		s.log("webhook delivery update failed", updateErr)
	}
}

func (s *WebhooksService) post(ctx context.Context, url string, body []byte, signature, deliveryID string) (int, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Template-Signature", signature)
	httpReq.Header.Set("X-Template-Delivery", deliveryID)
	var status int
	// A dead endpoint trips the webhooks.http breaker; while it is open
	// deliveries fail fast instead of queuing timeouts.
	err = s.breakers.Do(ctx, "webhooks.http", func() error {
		start := time.Now()
		resp, err := s.client.Do(httpReq)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		status = resp.StatusCode
		_ = time.Since(start)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return status, nil
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

func (s *WebhooksService) log(msg string, err error) {
	if s.logger != nil && err != nil {
		s.logger.Error(msg, "err", err)
	}
}
