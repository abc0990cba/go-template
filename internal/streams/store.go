package streams

import (
	"context"
	"time"

	"github.com/your-org/go-template/internal/kernel"
)

// Store applies enrichment events from the Kafka and NATS streams and
// manages the transactional outbox and the dead-letter queue.
// ApplyStreamEvent is exactly-once per (source, event_id) and atomic: dedupe
// check, per-source sequence guard, owned-fields merge, and the outbox write
// commit together. The in-memory fake and the PostgreSQL adapter both
// implement it; contract tests keep the two interchangeable.
type Store interface {
	ApplyStreamEvent(ctx context.Context, event Event) (ApplyResult, error)

	// StreamSeqs reports the applied per-source sequence numbers of a user;
	// the demo trigger uses them to publish the next one.
	StreamSeqs(ctx context.Context, userID string) (profile, contact int64, err error)

	// Outbox: Claim leases a batch (SKIP LOCKED + stale-lease reclaim), the
	// relay publishes outside the lease, then marks processed or releases
	// with an attempts bump. FailPermanently parks exhausted events.
	ClaimOutboxBatch(ctx context.Context, limit, leaseSeconds, maxAttempts int) ([]OutboxEvent, error)
	MarkOutboxProcessed(ctx context.Context, ids []string) error
	ReleaseOutboxBatch(ctx context.Context, ids []string) error
	FailOutboxPermanently(ctx context.Context, ids []string) error
	CountOutboxPending(ctx context.Context) (int, error)
	ListOutbox(ctx context.Context, opts kernel.ListOptions) ([]OutboxEvent, string, error)

	// Dead letters: poison or unprocessable messages parked for inspection.
	SaveDeadLetter(ctx context.Context, letter DeadLetter) error
	GetDeadLetter(ctx context.Context, id string) (DeadLetter, error)
	ListDeadLetters(ctx context.Context, opts kernel.DeadLetterOptions) ([]DeadLetter, string, error)
	MarkDeadLetterReplayed(ctx context.Context, id string) error
	CountDeadLettersPending(ctx context.Context) (int, error)

	// PruneProcessedEvents deletes inbox rows older than the retention
	// window. The inbox doubles as the exactly-once dedupe table, so the
	// window must comfortably exceed any redelivery delay the brokers can
	// produce; the worker sweeps it on a schedule.
	PruneProcessedEvents(ctx context.Context, olderThan time.Duration) (int64, error)
}
