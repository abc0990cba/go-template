//go:build integration

// The streams contract proves the in-memory fake and the PostgreSQL adapter
// apply enrichment events identically: exactly-once per (source, event_id),
// stale-sequence rejection, owned-column merges, and the atomic outbox row.
package integration

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/postgres"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/streams"
	"github.com/your-org/go-template/internal/users"
)

// jsonEqual compares a stored JSONB payload with the expected object,
// ignoring the whitespace normalization the database performs.
func jsonEqual(raw string, expected map[string]any) bool {
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return false
	}
	return reflect.DeepEqual(got, expected)
}

// streamsStore is the slice of the store surface the streams tests need.
type streamsStore interface {
	streams.Store
	kernel.IdempotencyRepo
	users.Store
}

func TestStreamsContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		runStreamsContract(t, repository.NewMemoryStore())
	})

	t.Run("postgres", func(t *testing.T) {
		ctx := context.Background()
		container, err := tcpostgres.Run(ctx,
			"postgres:16",
			tcpostgres.WithDatabase("app"),
			tcpostgres.WithUsername("app"),
			tcpostgres.WithPassword("app"),
			tcpostgres.BasicWaitStrategies(),
		)
		if err != nil {
			t.Fatalf("start postgres container: %v", err)
		}
		t.Cleanup(func() { _ = container.Terminate(ctx) })
		dsn, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatalf("connection string: %v", err)
		}
		store, err := postgres.Open(ctx, dsn, os.DirFS("../../migrations/postgres"))
		if err != nil {
			t.Fatalf("open store: %v", err)
		}
		t.Cleanup(store.Close)
		runStreamsContract(t, store)
	})
}

func runStreamsContract(t *testing.T, store streamsStore) {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	user := users.User{ID: "sc-u-1", Email: "streams@example.com", Name: "Streams", ExternalRef: "sc-ref-1", CreatedAt: now, UpdatedAt: now}
	if err := store.CreateUser(ctx, user, ""); err != nil {
		t.Fatalf("create user: %v", err)
	}

	event := func(source, eventID string, seq int64, payload string) streams.Event {
		return streams.Event{Source: source, EventID: eventID, ExternalRef: "sc-ref-1", Seq: seq, OccurredAt: now, Payload: payload}
	}

	t.Run("apply then dedupe then stale", func(t *testing.T) {
		result, err := store.ApplyStreamEvent(ctx, event(streams.SourceKafka, "ce-1", 5, `{"company":"Acme"}`))
		if err != nil || result.Status != streams.StatusApplied {
			t.Fatalf("apply = %+v err %v, want applied", result, err)
		}
		result, err = store.ApplyStreamEvent(ctx, event(streams.SourceKafka, "ce-1", 5, `{"company":"Overwritten"}`))
		if err != nil || result.Status != streams.StatusDeduped {
			t.Fatalf("redelivery = %+v err %v, want deduped", result, err)
		}
		result, err = store.ApplyStreamEvent(ctx, event(streams.SourceKafka, "ce-2", 4, `{"company":"Stale"}`))
		if err != nil || result.Status != streams.StatusStale {
			t.Fatalf("stale seq = %+v err %v, want stale", result, err)
		}
		got, err := store.GetUser(ctx, "sc-u-1")
		if err != nil {
			t.Fatalf("get user: %v", err)
		}
		// Postgres JSONB normalizes whitespace; compare as parsed JSON.
		if !jsonEqual(got.EnrichProfile, map[string]any{"company": "Acme"}) {
			t.Fatalf("enrich_profile = %q, want the first payload only", got.EnrichProfile)
		}
	})

	t.Run("both sources land without overwriting", func(t *testing.T) {
		result, err := store.ApplyStreamEvent(ctx, event(streams.SourceNats, "ce-3", 1, `{"phone":"+15550000"}`))
		if err != nil || result.Status != streams.StatusApplied {
			t.Fatalf("contact apply = %+v err %v", result, err)
		}
		got, _ := store.GetUser(ctx, "sc-u-1")
		if !jsonEqual(got.EnrichContact, map[string]any{"phone": "+15550000"}) ||
			!jsonEqual(got.EnrichProfile, map[string]any{"company": "Acme"}) {
			t.Fatalf("profile=%q contact=%q, want both sources preserved", got.EnrichProfile, got.EnrichContact)
		}
	})

	t.Run("outbox rows commit with the apply", func(t *testing.T) {
		pending, err := store.CountOutboxPending(ctx)
		if err != nil {
			t.Fatalf("count pending: %v", err)
		}
		if pending != 2 { // one per applied event; deduped/stale write nothing
			t.Fatalf("pending = %d, want 2", pending)
		}
		events, err := store.ClaimOutboxBatch(ctx, 10, 60, 5)
		if err != nil || len(events) != 2 {
			t.Fatalf("claim = %d events err %v, want 2", len(events), err)
		}
		if events[0].EventType != streams.OutboxUserEnriched || events[0].AggregateID != "sc-u-1" {
			t.Fatalf("claimed event = %+v", events[0])
		}
		if err := store.MarkOutboxProcessed(ctx, []string{events[0].ID}); err != nil {
			t.Fatalf("mark processed: %v", err)
		}
		if err := store.ReleaseOutboxBatch(ctx, []string{events[1].ID}); err != nil {
			t.Fatalf("release: %v", err)
		}
		pending, _ = store.CountOutboxPending(ctx)
		if pending != 1 {
			t.Fatalf("pending after settle = %d, want 1", pending)
		}
	})

	t.Run("unknown ref is ErrNotFound", func(t *testing.T) {
		_, err := store.ApplyStreamEvent(ctx, streams.Event{
			Source: streams.SourceKafka, EventID: "ce-x", ExternalRef: "nobody", Seq: 1, OccurredAt: now, Payload: `{}`,
		})
		if err == nil {
			t.Fatal("unknown ref: want error")
		}
	})

	t.Run("dead letters park and replay-flag", func(t *testing.T) {
		letter := streams.DeadLetter{ID: "dl-1", Source: streams.SourceKafka, Topic: "user-profile.v1", Payload: `{"bad`, Reason: streams.ReasonMalformed, Attempts: 1}
		if err := store.SaveDeadLetter(ctx, letter); err != nil {
			t.Fatalf("save: %v", err)
		}
		// Duplicate ids do not stack.
		if err := store.SaveDeadLetter(ctx, letter); err != nil {
			t.Fatalf("save duplicate: %v", err)
		}
		pending, err := store.CountDeadLettersPending(ctx)
		if err != nil || pending != 1 {
			t.Fatalf("pending letters = %d err %v, want 1", pending, err)
		}
		if err := store.MarkDeadLetterReplayed(ctx, "dl-1"); err != nil {
			t.Fatalf("mark replayed: %v", err)
		}
		if err := store.MarkDeadLetterReplayed(ctx, "dl-1"); err == nil {
			t.Fatal("double replay: want error")
		}
		got, err := store.GetDeadLetter(ctx, "dl-1")
		if err != nil || got.ReplayedAt == nil {
			t.Fatalf("letter = %+v err %v, want replayed stamp", got, err)
		}
	})

	t.Run("idempotency reserve is atomic", func(t *testing.T) {
		record := kernel.IdempotencyRecord{
			Key: "k-1", UserID: "sc-u-1", Method: "POST", Path: "/api/tickets",
			RequestHash: "hash-1", ExpiresAt: time.Now().Add(time.Hour),
		}
		stored, already, err := store.ReserveIdempotency(ctx, record)
		if err != nil || already {
			t.Fatalf("first reserve: already=%v err=%v, want fresh", already, err)
		}
		if stored.Status != 0 {
			t.Fatalf("fresh reservation status = %d, want 0 (in flight)", stored.Status)
		}
		stored, already, err = store.ReserveIdempotency(ctx, record)
		if err != nil || !already {
			t.Fatalf("second reserve: already=%v err=%v, want existing", already, err)
		}
		if err := store.CompleteIdempotency(ctx, "k-1", "sc-u-1", 201, []byte(`{"ok":true}`), "application/json"); err != nil {
			t.Fatalf("complete: %v", err)
		}
		stored, already, err = store.ReserveIdempotency(ctx, record)
		if err != nil || !already || stored.Status != 201 || string(stored.ResponseBody) != `{"ok":true}` {
			t.Fatalf("stored = %+v already=%v err=%v, want completed response", stored, already, err)
		}
		pruned, err := store.PruneIdempotency(ctx)
		if err != nil {
			t.Fatalf("prune: %v", err)
		}
		_ = pruned // the record is not expired yet; pruning just must not fail
	})

	// Deliberately last: it sweeps the inbox and applies a new event, so
	// earlier subtests' outbox/inbox counts would not survive it.
	t.Run("retention prune forgets the inbox, not the sequence guard", func(t *testing.T) {
		// A zero retention window sweeps every processed row. Redelivering
		// the same event must then fall through to the SEQUENCE guard
		// (stale) instead of the inbox (deduped) — proving which table
		// caught it. The inbox only accelerates dedupe; the seq columns
		// protect against re-application, which is why the retention window
		// may safely forget old rows.
		result, err := store.ApplyStreamEvent(ctx, event(streams.SourceKafka, "ce-1", 5, `{"company":"Acme"}`))
		if err != nil || result.Status != streams.StatusDeduped {
			t.Fatalf("pre-prune redelivery = %+v err %v, want deduped", result, err)
		}
		if n, err := store.PruneProcessedEvents(ctx, 0); err != nil || n == 0 {
			t.Fatalf("prune = %d rows, err %v; want the inbox swept", n, err)
		}
		result, err = store.ApplyStreamEvent(ctx, event(streams.SourceKafka, "ce-1", 5, `{"company":"Acme"}`))
		if err != nil || result.Status != streams.StatusStale {
			t.Fatalf("post-prune redelivery = %+v err %v, want stale via the seq guard", result, err)
		}
		// A longer window keeps fresh rows: the next sweep removes nothing.
		if _, err := store.ApplyStreamEvent(ctx, event(streams.SourceKafka, "ce-4", 6, `{"company":"Globex"}`)); err != nil {
			t.Fatalf("apply ce-4: %v", err)
		}
		if n, err := store.PruneProcessedEvents(ctx, time.Hour); err != nil || n != 0 {
			t.Fatalf("prune within window = %d rows, err %v; want none", n, err)
		}
	})
}
