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

	"github.com/redis/go-redis/v9"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/service"
)

// Channel is the Redis pub/sub channel every process publishes to.
const Channel = "app:realtime"

// Event types sent on the wire (SSE `event:` field).
const (
	TypeAnalyticsEvent = "analytics.event"
	TypeJobUpdated     = "job.updated"
	TypeNotification   = "notification.created"
	TypeSnapshot       = "snapshot"
	TypeConnected      = "connected"
)

// Event is one realtime message. Payload stays raw so the SSE layer can
// write it without re-marshalling.
type Event struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// NewEvent marshals payload once at publish time.
func NewEvent(eventType string, payload any) Event {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte("{}")
	}
	return Event{Type: eventType, Payload: data}
}

// Hub fans events out to subscribers. Publishing never blocks: a subscriber
// that cannot keep up misses events instead of stalling producers.
type Hub struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

// NewHub returns an empty hub.
func NewHub() *Hub {
	return &Hub{subs: map[chan Event]struct{}{}}
}

// Subscribe registers a new subscriber and returns its channel plus a
// cancel function that is safe to call more than once.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 16)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, ch)
			h.mu.Unlock()
		})
	}
}

// Publish delivers the event to every current subscriber, dropping it for
// any whose buffer is full.
func (h *Hub) Publish(ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
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

// FanSink is the service.EventSink the api process uses: it forwards every
// analytics event to the inner sinks (local trail, ClickHouse buffer) and
// publishes it to the hub for live delivery.
type FanSink struct {
	mu    sync.Mutex
	hub   *Hub
	inner []service.EventSink
}

// Register appends a late-wired inner sink; the api process uses it to hook
// the webhook fan-out in after the service container exists.
func (f *FanSink) Register(inner service.EventSink) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inner = append(f.inner, inner)
}

// NewFanSink wraps the given sinks; inner may be nil in tests.
func NewFanSink(hub *Hub, inner ...service.EventSink) *FanSink {
	return &FanSink{hub: hub, inner: inner}
}

// AddEvent implements service.EventSink.
func (f *FanSink) AddEvent(ctx context.Context, event, userID string) {
	f.mu.Lock()
	inner := make([]service.EventSink, len(f.inner))
	copy(inner, f.inner)
	f.mu.Unlock()
	for _, sink := range inner {
		sink.AddEvent(ctx, event, userID)
	}
	f.hub.Publish(NewEvent(TypeAnalyticsEvent, model.EventRecord{Name: event, OccurredAt: time.Now(), UserID: userID}))
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
// subscribe loop, the publisher, or both.
func NewRedisBridge(addr string, hub *Hub, logger *slog.Logger) *RedisBridge {
	return &RedisBridge{rdb: redis.NewClient(&redis.Options{Addr: addr}), hub: hub, logger: logger}
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
	Webhooks service.WebhookFan
}

// AddEvent implements service.EventSink.
func (s WebhookSink) AddEvent(ctx context.Context, event, userID string) {
	s.Webhooks.HandleEvent(ctx, event, userID)
}
