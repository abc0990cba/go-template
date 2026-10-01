// External test package: the repository memory store implements streams'
// Store, so a same-package test importing it would cycle.
package streams_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/reliability"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/streams"
	"github.com/your-org/go-template/internal/users"
)

func testStreams(t *testing.T) (*streams.Service, *repository.MemoryStore) {
	t.Helper()
	store := repository.NewMemoryStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return streams.NewService(store, nil, logger), store
}

func mustUser(t *testing.T, store *repository.MemoryStore, ref string) {
	t.Helper()
	now := time.Now()
	user := users.User{ID: "u-" + ref, Email: ref + "@example.com", Name: ref, ExternalRef: ref, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateUser(context.Background(), user, ""); err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func profilePayload(fields ...string) string {
	pairs := make([]string, 0, len(fields))
	for _, f := range fields {
		pairs = append(pairs, fmt.Sprintf("%q:%q", f, f+"-value"))
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

func TestApplyProfileAndContactMergeIndependently(t *testing.T) {
	svc, store := testStreams(t)
	ctx := context.Background()
	mustUser(t, store, "demo-u-001")

	if err := svc.HandleProfileEvent(ctx, "user-profile.v1", []byte(fmt.Sprintf(
		`{"event_id":"k1","external_ref":"demo-u-001","seq":1,"data":%s}`, profilePayload("company", "job_title"))), nil); err != nil {
		t.Fatalf("profile event: %v", err)
	}
	if err := svc.HandleContactEvent(ctx, "users.contact.v1", []byte(
		`{"event_id":"n1","external_ref":"demo-u-001","seq":1,"data":{"phone":"+1 555 0100"}}`), nil); err != nil {
		t.Fatalf("contact event: %v", err)
	}

	user, err := store.GetUser(ctx, "u-demo-u-001")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !strings.Contains(user.EnrichProfile, `"company"`) || !strings.Contains(user.EnrichProfile, `"job_title"`) {
		t.Fatalf("profile fields missing: %s", user.EnrichProfile)
	}
	if !strings.Contains(user.EnrichContact, `"phone"`) {
		t.Fatalf("contact fields missing: %s", user.EnrichContact)
	}
	if user.EnrichedAt == nil {
		t.Fatal("enriched_at not set")
	}
}

func TestApplyExactlyOnceAndStaleGuard(t *testing.T) {
	svc, store := testStreams(t)
	ctx := context.Background()
	mustUser(t, store, "ref-1")

	apply := func(eventID string, seq int, company string) error {
		return svc.HandleProfileEvent(ctx, "user-profile.v1", []byte(fmt.Sprintf(
			`{"event_id":%q,"external_ref":"ref-1","seq":%d,"data":{"company":%q}}`, eventID, seq, company)), nil)
	}
	if err := apply("e1", 5, "first"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Same event redelivered: deduped, payload untouched.
	if err := apply("e1", 5, "overwritten-by-replay"); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	// Older seq arriving late: stale, ignored.
	if err := apply("e2", 4, "stale-write"); err != nil {
		t.Fatalf("stale: %v", err)
	}
	user, _ := store.GetUser(ctx, "u-ref-1")
	if user.EnrichProfile == "" || strings.Contains(user.EnrichProfile, "overwritten-by-replay") || strings.Contains(user.EnrichProfile, "stale-write") {
		t.Fatalf("exactly-once violated: %q", user.EnrichProfile)
	}
	if !strings.Contains(user.EnrichProfile, `"first"`) {
		t.Fatalf("expected first payload: %q", user.EnrichProfile)
	}

	pending, err := store.CountOutboxPending(ctx)
	if err != nil || pending != 1 {
		t.Fatalf("outbox pending = %d err %v, want exactly one event", pending, err)
	}
}

func TestApplyMalformedEventIsPermanent(t *testing.T) {
	svc, _ := testStreams(t)
	err := svc.HandleProfileEvent(context.Background(), "user-profile.v1", []byte(`{"event_id":`), nil)
	var permanent reliability.PermanentError
	if !errors.As(err, &permanent) {
		t.Fatalf("err = %v, want PermanentError", err)
	}
	// A valid envelope with a broken data payload is also permanent.
	err = svc.HandleProfileEvent(context.Background(), "user-profile.v1", []byte(`{"event_id":"x","external_ref":"r","seq":1,"data":[1,2]}`), nil)
	if !errors.As(err, &permanent) {
		t.Fatalf("non-object data: err = %v, want PermanentError", err)
	}
}

func TestApplyUnknownUserRetriesThenSignals(t *testing.T) {
	svc, store := testStreams(t)
	ctx := context.Background()
	svc.WithUnknownUserRetry(20, 20*time.Millisecond)

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.HandleProfileEvent(ctx, "user-profile.v1", []byte(
			`{"event_id":"late-1","external_ref":"late-ref","seq":1,"data":{"company":"x"}}`), nil)
	}()
	// The user appears while the service is still retrying.
	time.Sleep(100 * time.Millisecond)
	mustUser(t, store, "late-ref")

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("late user never applied: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for apply")
	}
	user, err := store.GetUser(ctx, "u-late-ref")
	if err != nil || !strings.Contains(user.EnrichProfile, "company") {
		t.Fatalf("profile not applied: %q err %v", user.EnrichProfile, err)
	}
}

func TestConcurrentSourcesNoLostUpdates(t *testing.T) {
	svc, store := testStreams(t)
	ctx := context.Background()
	mustUser(t, store, "race-ref")

	// Two external systems deliver concurrently for the same user: each
	// source streams its own events in order (Kafka partitions by key and
	// JetStream durable consumers preserve source order), while the other
	// source interleaves. Field ownership plus the serialize-then-merge
	// apply must land every field exactly once — and concurrent duplicate
	// deliveries of the same event must not double-apply.
	const perSource = 40
	var wg sync.WaitGroup
	for _, src := range []string{"kafka", "nats"} {
		wg.Add(1)
		go func(source string) {
			defer wg.Done()
			prefix, handler, topic := "k", svc.HandleProfileEvent, "user-profile.v1"
			if source == "nats" {
				prefix, handler, topic = "n", svc.HandleContactEvent, "users.contact.v1"
			}
			for i := range perSource {
				payload := fmt.Sprintf(`{"event_id":"%s-%d","external_ref":"race-ref","seq":%d,"data":{"%s%d":%q}}`,
					prefix, i, i+1, prefix, i, fmt.Sprint(i))
				// Deliver every event twice, as at-least-once brokers do
				// under retries; the second copy must be a no-op.
				for delivery := range 2 {
					if err := handler(ctx, topic, []byte(payload), nil); err != nil {
						t.Errorf("%s event %d delivery %d: %v", source, i, delivery, err)
					}
				}
			}
		}(src)
	}
	wg.Wait()

	user, err := store.GetUser(ctx, "u-race-ref")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	missing := 0
	for i := range perSource {
		if !strings.Contains(user.EnrichProfile, fmt.Sprintf(`"k%d"`, i)) {
			missing++
		}
		if !strings.Contains(user.EnrichContact, fmt.Sprintf(`"n%d"`, i)) {
			missing++
		}
	}
	if missing > 0 {
		t.Fatalf("lost updates: %d of %d fields missing\nprofile: %s\ncontact: %s", missing, perSource*2, user.EnrichProfile, user.EnrichContact)
	}
	// Every applied event commits its outbox row with the merge, so the
	// relay queue must hold exactly one user.enriched per event.
	pending, _ := store.CountOutboxPending(ctx)
	if pending != perSource*2 {
		t.Fatalf("outbox events = %d, want %d", pending, perSource*2)
	}
}

func TestRelayOutboxOncePublishesAndMarks(t *testing.T) {
	svc, store := testStreams(t)
	ctx := context.Background()
	mustUser(t, store, "relay-ref")

	_ = svc.HandleProfileEvent(ctx, "user-profile.v1", []byte(
		`{"event_id":"r1","external_ref":"relay-ref","seq":1,"data":{"company":"Acme"}}`), nil)

	var published []streams.OutboxEvent
	pub := fakePublisher{onPublish: func(e streams.OutboxEvent) error {
		published = append(published, e)
		return nil
	}}
	n, err := svc.RelayOutboxOnce(ctx, pub, streams.RelayConfig{Batch: 10, LeaseSeconds: 60, MaxAttempts: 3})
	if err != nil || n != 1 {
		t.Fatalf("relay: n=%d err=%v", n, err)
	}
	if len(published) != 1 || published[0].EventType != streams.OutboxUserEnriched {
		t.Fatalf("published = %+v", published)
	}
	pending, _ := store.CountOutboxPending(ctx)
	if pending != 0 {
		t.Fatalf("outbox not drained: %d", pending)
	}
	// The relay pass is idempotent: an empty queue relays nothing.
	n, err = svc.RelayOutboxOnce(ctx, pub, streams.RelayConfig{Batch: 10, LeaseSeconds: 60, MaxAttempts: 3})
	if err != nil || n != 0 {
		t.Fatalf("second relay: n=%d err=%v", n, err)
	}
}

func TestRelayOutboxOnceRetriesThenDeadLetters(t *testing.T) {
	svc, store := testStreams(t)
	ctx := context.Background()
	mustUser(t, store, "dlq-ref")
	_ = svc.HandleProfileEvent(ctx, "user-profile.v1", []byte(
		`{"event_id":"d1","external_ref":"dlq-ref","seq":1,"data":{"company":"X"}}`), nil)

	// Fail every publish until attempts are exhausted.
	fail := true
	pub := fakePublisher{onPublish: func(_ streams.OutboxEvent) error {
		if fail {
			return errors.New("broker down")
		}
		return nil
	}}
	cfg := streams.RelayConfig{Batch: 10, LeaseSeconds: 0, MaxAttempts: 2}
	if _, err := svc.RelayOutboxOnce(ctx, pub, cfg); err != nil {
		t.Fatalf("relay 1: %v", err)
	}
	if _, err := svc.RelayOutboxOnce(ctx, pub, cfg); err != nil {
		t.Fatalf("relay 2: %v", err)
	}
	letters, _, err := store.ListDeadLetters(ctx, kernel.DeadLetterOptions{Pending: true})
	if err != nil || len(letters) != 1 {
		t.Fatalf("dead letters = %d err %v, want 1", len(letters), err)
	}
	if letters[0].Reason != streams.ReasonMaxAttempts || letters[0].Source != "outbox" {
		t.Fatalf("letter = %+v", letters[0])
	}
	pending, _ := store.CountOutboxPending(ctx)
	if pending != 0 {
		t.Fatalf("exhausted event still pending: %d", pending)
	}

	// Once the broker recovers, replaying the letter re-publishes it.
	fail = false
	svc.WithRelayPublisher(pub)
	if _, err := svc.ReplayDeadLetter(ctx, letters[0].ID); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if _, err := svc.ReplayDeadLetter(ctx, letters[0].ID); !errors.Is(err, streams.ErrAlreadyReplayed) {
		t.Fatalf("second replay err = %v, want ErrAlreadyReplayed", err)
	}
}

func TestParkUnknownUserAndReplayAfterUserArrives(t *testing.T) {
	svc, store := testStreams(t)
	ctx := context.Background()
	svc.WithUnknownUserRetry(1, 200*time.Millisecond) // park immediately

	payload := []byte(`{"event_id":"park-1","external_ref":"parked-ref","seq":1,"data":{"company":"Y"}}`)
	err := svc.HandleProfileEvent(ctx, "user-profile.v1", payload, nil)
	if !errors.Is(err, streams.ErrUnknownUser) {
		t.Fatalf("err = %v, want ErrUnknownUser", err)
	}
	letters, _, _ := store.ListDeadLetters(ctx, kernel.DeadLetterOptions{Pending: true})
	if len(letters) != 1 || letters[0].Reason != streams.ReasonUnknownUser {
		t.Fatalf("letters = %+v", letters)
	}
	// User shows up; replay lands the event.
	mustUser(t, store, "parked-ref")
	if _, err := svc.ReplayDeadLetter(ctx, letters[0].ID); err != nil {
		t.Fatalf("replay: %v", err)
	}
	user, _ := store.GetUser(ctx, "u-parked-ref")
	if !strings.Contains(user.EnrichProfile, "company") {
		t.Fatalf("replayed payload missing: %q", user.EnrichProfile)
	}
}

type fakePublisher struct {
	onPublish func(streams.OutboxEvent) error
}

func (f fakePublisher) PublishUserEvent(_ context.Context, event streams.OutboxEvent) error {
	return f.onPublish(event)
}
