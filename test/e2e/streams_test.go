//go:build e2e

// Enrichment round-trip against the brokers stack (make demo-up): publish a
// profile event into Kafka and a contact event into NATS, then watch the
// user row merge both payloads. Skips when no broker is reachable, so the
// plain e2e suite keeps working without the streams.
package e2e

import ()

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	e2eKafkaBrokers   = "localhost:9092"
	e2eNatsURL        = "nats://localhost:4222"
	e2eProfileTopic   = "user-profile.v1"
	e2eContactSubject = "users.contact.v1"
)

func TestEnrichmentRoundTrip(t *testing.T) {
	requireStack(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ref := fmt.Sprintf("e2e-streams-%d", time.Now().UnixMilli())
	publishKafkaE2E(t, ctx, ref)
	publishNATSE2E(t, ctx, ref)

	// The demo user with this external_ref does not exist: create it via the
	// API carrying the ref, then wait for the streams to merge both payloads.
	client := &http.Client{Timeout: 10 * time.Second}
	token := loginAdmin(t, client)
	status, body := call(t, client, http.MethodPost, "/api/users",
		fmt.Sprintf(`{"email":"%s@example.com","name":"Streams E2E","external_ref":%q}`, ref, ref), token)
	if status != http.StatusCreated {
		t.Fatalf("create user: want 201, got %d (%v)", status, body)
	}
	userID, _ := body["id"].(string)

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		status, body := call(t, client, http.MethodGet, "/api/users/"+userID, "", token)
		if status == http.StatusOK {
			profile, _ := body["enrich_profile"].(string)
			contact, _ := body["enrich_contact"].(string)
			if strings.Contains(profile, "E2E Corp") && strings.Contains(contact, "+1555E2E") {
				t.Logf("merged enrichment visible for %s: profile=%s contact=%s", ref, profile, contact)
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("enrichment did not land within 45s (is the worker running with the brokers enabled?)")
}

func loginAdmin(t *testing.T, client *http.Client) string {
	t.Helper()
	status, body := call(t, client, http.MethodPost, "/api/auth/login",
		`{"email":"demo@go-template.local","password":"demo-password-123"}`, "")
	if status != http.StatusOK {
		t.Skipf("demo login unavailable: status %d", status)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Skip("demo login returned no token")
	}
	return token
}

// probeBroker skips the test when the stream infrastructure is not up.
func probeBroker(t *testing.T) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(e2eKafkaBrokers))
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		err = cl.Ping(ctx)
		cl.Close()
	}
	if err != nil {
		t.Skipf("kafka not reachable at %s (run `make demo-up`): %v", e2eKafkaBrokers, err)
	}
}

func publishKafkaE2E(t *testing.T, ctx context.Context, ref string) {
	t.Helper()
	probeBroker(t)
	env := map[string]any{
		"event_id":     ref + "-profile",
		"external_ref": ref,
		"seq":          1,
		"occurred_at":  time.Now().UTC().Format(time.RFC3339Nano),
		"data":         map[string]any{"company": "E2E Corp", "enriched_by": "e2e"},
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	cl, err := kgo.NewClient(kgo.SeedBrokers(e2eKafkaBrokers))
	if err != nil {
		t.Fatalf("kafka client: %v", err)
	}
	defer cl.Close()
	rec := &kgo.Record{Topic: e2eProfileTopic, Key: []byte(ref), Value: raw}
	acked := make(chan error, 1)
	cl.Produce(ctx, rec, func(_ *kgo.Record, err error) { acked <- err })
	select {
	case err := <-acked:
		if err != nil {
			t.Fatalf("publish: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("publish timed out")
	}
}

func publishNATSE2E(t *testing.T, ctx context.Context, ref string) {
	t.Helper()
	nc, err := nats.Connect(e2eNatsURL, nats.Timeout(3*time.Second))
	if err != nil {
		t.Skipf("nats not reachable at %s (run `make demo-up`): %v", e2eNatsURL, err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Skipf("nats jetstream unavailable: %v", err)
	}
	env := map[string]any{
		"event_id":     ref + "-contact",
		"external_ref": ref,
		"seq":          1,
		"occurred_at":  time.Now().UTC().Format(time.RFC3339Nano),
		"data":         map[string]any{"phone": "+1555E2E", "enriched_by": "e2e"},
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := js.PublishAsync(e2eContactSubject, raw); err != nil {
		t.Fatalf("nats publish: %v", err)
	}
	select {
	case <-js.PublishAsyncComplete():
	case <-ctx.Done():
		t.Fatal("nats publish timed out")
	}
}
