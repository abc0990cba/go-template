// External test package: the memory store (repository) implements
// webhooks.Store, so a same-package test importing it would cycle.
package webhooks_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/webhooks"
)

// flagSettings answers the webhooks.enabled feature check.
type flagSettings struct{ enabled bool }

func (f *flagSettings) Bool(_ context.Context, _ string) bool { return f.enabled }

// receiver is a webhook target capturing what was posted.
type receiver struct {
	mu       sync.Mutex
	requests []capturedRequest
	status   int
}

type capturedRequest struct {
	body      string
	signature string
	delivery  string
}

func (r *receiver) handler(status *int) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.requests = append(r.requests, capturedRequest{
			body:      string(body),
			signature: req.Header.Get("X-Template-Signature"),
			delivery:  req.Header.Get("X-Template-Delivery"),
		})
		r.mu.Unlock()
		w.WriteHeader(*status)
	}
}

func (r *receiver) last() capturedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requests[len(r.requests)-1]
}

func (r *receiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

type fixture struct {
	svc             *webhooks.Service
	store           *repository.MemoryStore
	target          *receiver
	status          *int
	setting         *flagSettings
	targetServerURL string
}

// newFixture wires the service over the memory store with a synchronous
// (dispatcher-less) delivery path aimed at a test receiver.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	store := repository.NewMemoryStore()
	target := &receiver{}
	status := http.StatusOK
	mux := http.NewServeMux()
	mux.HandleFunc("/hook", target.handler(&status))
	targetServer := httptest.NewServer(mux)
	t.Cleanup(targetServer.Close)
	settings := &flagSettings{enabled: true}
	svc := webhooks.NewService(store, nil, settings, targetServer.Client(), nil, nil, discardLogger())
	return &fixture{svc: svc, store: store, target: target, status: &status, setting: settings, targetServerURL: targetServer.URL}
}

func (f *fixture) createEndpoint(t *testing.T, events ...string) webhooks.Endpoint {
	t.Helper()
	endpoint, err := f.svc.Create(context.Background(), webhooks.Endpoint{
		URL: f.targetURL(), Events: events, Active: true,
	})
	if err != nil {
		t.Fatalf("create endpoint: %v", err)
	}
	if endpoint.Secret == "" {
		t.Fatal("create endpoint: want generated secret")
	}
	return endpoint
}

func (f *fixture) targetURL() string { return f.targetServerURL + "/hook" }

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestEndpointCRUDValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	for _, bad := range []string{"", "ftp://example.com/hook", "https://", "/relative/only"} {
		if _, err := f.svc.Create(ctx, webhooks.Endpoint{URL: bad, Active: true}); !kernel.IsValidationError(err) {
			t.Errorf("create with url %q: want validation error, got %v", bad, err)
		}
	}

	endpoint := f.createEndpoint(t, "user.created")
	// A provided secret is kept, not replaced.
	kept, err := f.svc.Create(ctx, webhooks.Endpoint{
		URL: f.targetURL(), Events: []string{"*"}, Active: true, Secret: "my-own-secret",
	})
	if err != nil {
		t.Fatalf("create with secret: %v", err)
	}
	if kept.Secret != "my-own-secret" {
		t.Errorf("provided secret: want kept, got %q", kept.Secret)
	}

	// Update rewrites the mutable fields and leaves id and secret alone.
	endpoint.URL = f.targetServerURL + "/moved"
	endpoint.Events = []string{"ticket.approved"}
	endpoint.Description = "moved"
	if err := f.svc.Update(ctx, endpoint); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated, err := f.svc.Get(ctx, endpoint.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.URL != f.targetServerURL+"/moved" || updated.Secret != endpoint.Secret || updated.Description != "moved" {
		t.Errorf("update: url/secret/description changed unexpectedly: %+v", updated)
	}

	if err := f.svc.Delete(ctx, endpoint.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := f.svc.Get(ctx, endpoint.ID); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("get after delete: want ErrNotFound, got %v", err)
	}
}

func TestDeliverySignsAndRecordsSuccess(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	endpoint := f.createEndpoint(t, "user.created")

	f.svc.HandleEvent(ctx, "user.created", "u1")

	if f.target.count() != 1 {
		t.Fatalf("delivery: want 1 post, got %d", f.target.count())
	}
	got := f.target.last()

	// The golden signature: HMAC-SHA256 of the exact body with the
	// endpoint secret, hex-encoded behind the sha256= prefix.
	var payload map[string]any
	if err := json.Unmarshal([]byte(got.body), &payload); err != nil {
		t.Fatalf("payload is not json: %v (%q)", err, got.body)
	}
	if payload["event"] != "user.created" || payload["user_id"] != "u1" {
		t.Errorf("payload: want user.created/u1, got %v", payload)
	}
	mac := hmac.New(sha256.New, []byte(endpoint.Secret))
	mac.Write([]byte(got.body))
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got.signature != want {
		t.Errorf("signature: want %s, got %s", want, got.signature)
	}

	// The delivery log recorded the success with the same id the receiver saw.
	deliveries, err := f.svc.Deliveries(ctx, endpoint.ID, 10, 0)
	if err != nil {
		t.Fatalf("deliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("delivery log: want 1 entry, got %d", len(deliveries))
	}
	delivery := deliveries[0]
	if delivery.Status != "success" || delivery.Attempts != 1 || delivery.DeliveredAt == nil {
		t.Errorf("delivery record: want success/1 attempt/delivered, got %+v", delivery)
	}
	if delivery.ID != got.delivery {
		t.Errorf("delivery id: receiver saw %s, log says %s", got.delivery, delivery.ID)
	}
}

func TestHandleEventMatchingAndFlag(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// One exact-match endpoint and one wildcard endpoint.
	wildcard := f.createEndpoint(t, "*")
	specific := f.createEndpoint(t, "ticket.approved")
	if wildcard.ID == specific.ID {
		t.Fatal("endpoints must differ")
	}

	f.svc.HandleEvent(ctx, "user.created", "u1")
	if f.target.count() != 1 {
		t.Fatalf("fan-out: want 1 delivery (wildcard only), got %d", f.target.count())
	}

	// The specific endpoint ignores unrelated events.
	f.svc.HandleEvent(ctx, "ticket.approved", "u1")
	if f.target.count() != 3 {
		t.Fatalf("fan-out: want 3 deliveries total, got %d", f.target.count())
	}

	// Inactive endpoints are skipped.
	if err := f.svc.Update(ctx, webhooks.Endpoint{ID: wildcard.ID, URL: f.targetURL(), Events: []string{"*"}, Active: false}); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	f.svc.HandleEvent(ctx, "user.created", "u1")
	if f.target.count() != 3 {
		t.Errorf("inactive endpoint should not receive events: got %d", f.target.count())
	}

	// The feature flag switches the fan-out off entirely.
	f.setting.enabled = false
	f.svc.HandleEvent(ctx, "ticket.approved", "u1")
	if f.target.count() != 3 {
		t.Errorf("disabled flag should stop fan-out: got %d", f.target.count())
	}
}

func TestFailedDeliveryRetriesThroughQueueReplay(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	endpoint := f.createEndpoint(t, "user.created")

	// A 500 records the failure with the upstream status.
	*f.status = http.StatusInternalServerError
	f.svc.HandleEvent(ctx, "user.created", "u1")
	deliveries, err := f.svc.Deliveries(ctx, endpoint.ID, 10, 0)
	if err != nil {
		t.Fatalf("deliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("delivery log: want 1 entry, got %d", len(deliveries))
	}
	failed := deliveries[0]
	if failed.Status != "failed" || failed.LastResponseStatus != http.StatusInternalServerError {
		t.Fatalf("failed delivery: want failed/500, got %+v", failed)
	}
	if failed.DeliveredAt != nil {
		t.Errorf("failed delivery must have no DeliveredAt: %+v", failed)
	}

	// The queue worker's re-run posts again and turns the delivery green.
	*f.status = http.StatusOK
	if err := f.svc.DeliverQueued(ctx, endpoint.ID, failed.ID); err != nil {
		t.Fatalf("deliver queued: %v", err)
	}
	deliveries, _ = f.svc.Deliveries(ctx, endpoint.ID, 10, 0)
	retried := deliveries[0]
	if retried.Status != "success" || retried.Attempts != 2 {
		t.Errorf("retried delivery: want success/2 attempts, got %+v", retried)
	}

	// An unknown delivery id is ErrNotFound.
	if err := f.svc.DeliverQueued(ctx, endpoint.ID, "no-such-delivery"); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("unknown delivery replay: want ErrNotFound, got %v", err)
	}
}

func TestSendTestFiresSyntheticEvent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	endpoint := f.createEndpoint(t, "*")

	if err := f.svc.SendTest(ctx, endpoint.ID); err != nil {
		t.Fatalf("send test: %v", err)
	}
	var payload map[string]any
	_ = json.Unmarshal([]byte(f.target.last().body), &payload)
	if payload["event"] != "webhook.test" {
		t.Errorf("send test payload: want webhook.test, got %v", payload)
	}

	if err := f.svc.SendTest(ctx, "no-such-endpoint"); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("send test to unknown endpoint: want ErrNotFound, got %v", err)
	}
}
