// Package realtime carries live updates from the backend to the browser
// over Server-Sent Events: analytics events as they are emitted, job status
// transitions, and periodic snapshots. The Hub fans in-process events out to
// SSE subscribers; the Redis bridge relays events published by other
// processes (the worker) into the same hub.
package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/webhooks"
)

// Channel is the Redis pub/sub channel every process publishes to.
const Channel = "app:realtime"

// Event types sent on the wire (SSE `event:` field).
const (
	TypeAnalyticsEvent = "analytics.event"
	TypeJobUpdated     = "job.updated"
	TypeTicketUpdated  = "ticket.updated"
	TypeNotification   = "notification.created"
	TypeSnapshot       = "snapshot"
	TypeConnected      = "connected"
)

// Event is one realtime message. Payload stays raw so the SSE layer can
// write it without re-marshalling. The Route rides along the Redis bridge
// (process → process) but the SSE/WS writers strip it — clients see only
// Type and Payload.
type Event struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	Route   Route           `json:"route,omitempty"`
}

// Route scopes who may receive an event. The zero value is "everyone
// authenticated" (the connected event). When several fields are set, User
// wins over AdminOnly wins over Org.
type Route struct {
	// Org delivers to subscribers whose Orgs contain this domain;
	// platform admins always pass.
	Org string `json:"org,omitempty"`
	// User delivers to exactly this subscriber — personal events such as
	// notifications, even from admins.
	User string `json:"user,omitempty"`
	// AdminOnly delivers to platform admins only.
	AdminOnly bool `json:"admin_only,omitempty"`
}

// WireEvent is the client-visible shape: routing never leaves the server.
type WireEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Wire strips the routing metadata for the client wire. The SSE/WS writers
// marshal this, never the full Event: org scopes and user targets stay on
// the server.
func (e Event) Wire() WireEvent { return WireEvent{Type: e.Type, Payload: e.Payload} }

// NewEvent marshals payload once at publish time; everyone authenticated
// receives it.
func NewEvent(eventType string, payload any) Event {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte("{}")
	}
	return Event{Type: eventType, Payload: data}
}

// NewOrgEvent scopes delivery to one organization's members (platform
// admins included).
func NewOrgEvent(eventType string, payload any, org string) Event {
	ev := NewEvent(eventType, payload)
	ev.Route.Org = org
	return ev
}

// NewUserEvent scopes delivery to a single user — notifications are
// personal even when an admin triggers them.
func NewUserEvent(eventType string, payload any, userID string) Event {
	ev := NewEvent(eventType, payload)
	ev.Route.User = userID
	return ev
}

// NewAdminEvent scopes delivery to platform admins.
func NewAdminEvent(eventType string, payload any) Event {
	ev := NewEvent(eventType, payload)
	ev.Route.AdminOnly = true
	return ev
}

// Subscriber carries the identity a connection receives events for,
// resolved once at subscribe time from the authenticated request.
type Subscriber struct {
	// UserID is the authenticated principal; personal events match it.
	UserID string
	// Orgs lists the subscriber's organization memberships; org-routed
	// events must hit one of them.
	Orgs []string
	// Admin marks platform-wide administrators (cross-org visibility).
	Admin bool
	// CanReadJobs gates the periodic snapshot, mirroring the jobs-list
	// REST permission.
	CanReadJobs bool
}

// visibleTo applies the event's route to the subscriber.
func (e Event) visibleTo(sub Subscriber) bool {
	switch {
	case e.Route.User != "":
		return e.Route.User == sub.UserID
	case e.Route.AdminOnly:
		return sub.Admin
	case e.Route.Org != "":
		if sub.Admin {
			return true
		}
		for _, org := range sub.Orgs {
			if org == e.Route.Org {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// subscription pairs a subscriber with its delivery channel.
type subscription struct {
	sub Subscriber
	ch  chan Event
}

// Hub fans events out to subscribers, honoring each event's route. A
// subscriber that cannot keep up misses events instead of stalling
// producers — and never sees events outside its scope.
type Hub struct {
	mu   sync.RWMutex
	subs map[*subscription]struct{}
}

// NewHub returns an empty hub.
func NewHub() *Hub {
	return &Hub{subs: map[*subscription]struct{}{}}
}

// Subscribe registers an authenticated subscriber and returns its channel
// plus a cancel function that is safe to call more than once.
func (h *Hub) Subscribe(sub Subscriber) (<-chan Event, func()) {
	entry := &subscription{sub: sub, ch: make(chan Event, 16)}
	h.mu.Lock()
	h.subs[entry] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return entry.ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, entry)
			h.mu.Unlock()
		})
	}
}

// Publish delivers the event to every current subscriber in scope, dropping
// it for any whose buffer is full.
func (h *Hub) Publish(ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for entry := range h.subs {
		if !ev.visibleTo(entry.sub) {
			continue
		}
		select {
		case entry.ch <- ev:
		default:
		}
	}
}

// Subscribers reports the current fan-out width (tests and diagnostics).
func (h *Hub) Subscribers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

// FanSink is the analytics.Sink the api process uses: it forwards every
// analytics event to the inner sinks (local trail, ClickHouse buffer) and
// publishes it to the hub for live delivery. Activity events are routed to
// the actor's organization (admins see everything); without a resolver they
// degrade to admin-only rather than leaking activity across orgs.
type FanSink struct {
	mu    sync.Mutex
	hub   *Hub
	inner []analytics.Sink
	orgs  func(ctx context.Context, userID string) []string
}

// WithOrgResolver attaches the actor → organizations lookup.
func (f *FanSink) WithOrgResolver(resolve func(ctx context.Context, userID string) []string) *FanSink {
	f.orgs = resolve
	return f
}

// Register appends a late-wired inner sink; the api process uses it to hook
// the webhook fan-out in after the service container exists.
func (f *FanSink) Register(inner analytics.Sink) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inner = append(f.inner, inner)
}

// NewFanSink wraps the given sinks; inner may be nil in tests.
func NewFanSink(hub *Hub, inner ...analytics.Sink) *FanSink {
	return &FanSink{hub: hub, inner: inner}
}

// AddEvent implements analytics.Sink.
func (f *FanSink) AddEvent(ctx context.Context, event, userID string) {
	f.mu.Lock()
	inner := make([]analytics.Sink, len(f.inner))
	copy(inner, f.inner)
	orgs := f.orgs
	f.mu.Unlock()
	for _, sink := range inner {
		sink.AddEvent(ctx, event, userID)
	}
	ev := NewEvent(TypeAnalyticsEvent, analytics.EventRecord{Name: event, OccurredAt: time.Now(), UserID: userID})
	if orgs != nil {
		if actorOrgs := orgs(ctx, userID); len(actorOrgs) > 0 {
			for _, org := range actorOrgs {
				// Deliver per org: one actor, all their orgs see the event.
				routed := ev
				routed.Route.Org = org
				f.hub.Publish(routed)
			}
			return
		}
	}
	// No resolver (tests, platform-only actors): admins only.
	f.hub.Publish(NewAdminEvent(TypeAnalyticsEvent, analytics.EventRecord{Name: event, OccurredAt: time.Now(), UserID: userID}))
}

// RedisBridge relays events between the api and worker processes. The worker
// publishes job transitions; the api subscribes and republishes into its
// hub so browsers see worker activity live.
type RedisBridge struct {
	rdb    *redis.Client
	hub    *Hub
	logger *slog.Logger
}

// NewRedisBridge connects to Redis; the caller decides whether to run the
// subscribe loop, the publisher, or both. Commands get OpenTelemetry client
// spans so pub/sub round-trips appear in traces.
func NewRedisBridge(addr string, hub *Hub, logger *slog.Logger) *RedisBridge {
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := redisotel.InstrumentTracing(client); err != nil {
		logger.Debug("redis tracing unavailable", "err", err)
	}
	return &RedisBridge{rdb: client, hub: hub, logger: logger}
}

// Run subscribes to the realtime channel and republishes every message into
// the hub. It blocks until ctx is done; call it in a goroutine.
func (b *RedisBridge) Run(ctx context.Context) {
	sub := b.rdb.Subscribe(ctx, Channel)
	defer func() { _ = sub.Close() }()
	for msg := range sub.Channel() {
		var ev Event
		if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
			b.logger.Warn("realtime relay dropped a malformed message", "err", err)
			continue
		}
		b.hub.Publish(ev)
	}
}

// Publish sends an event to the Redis channel for the other processes.
func (b *RedisBridge) Publish(ev Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	if err := b.rdb.Publish(context.Background(), Channel, data).Err(); err != nil {
		b.logger.Warn("realtime publish failed", "err", err)
	}
}

// Close releases the Redis connection.
func (b *RedisBridge) Close() error { return b.rdb.Close() }

// WebhookSink adapts the webhook fan-out to the EventSink shape so the
// fan sink can forward analytics events to subscribed endpoints.
type WebhookSink struct {
	Webhooks webhooks.Fan
}

// AddEvent implements analytics.Sink.
func (s WebhookSink) AddEvent(ctx context.Context, event, userID string) {
	s.Webhooks.HandleEvent(ctx, event, userID)
}
