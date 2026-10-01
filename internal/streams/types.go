package streams

import "time"

// Enrichment stream sources. Each source owns exactly one column of the
// users row, so concurrent events from Kafka and NATS never overwrite each
// other's fields — the row lock serializes them, ownership makes that safe.
const (
	SourceKafka = "kafka"
	SourceNats  = "nats"
)

// Event is one user-enrichment message from an external system. EventID
// is globally unique per source (the dedupe key), Seq is the source's
// per-user monotonic sequence, and Payload is a raw JSON object of the
// fields this source owns — merged as-is into the user's JSONB column.
// Headers carry propagated trace context.
type Event struct {
	Source      string            `json:"source"`
	EventID     string            `json:"event_id"`
	ExternalRef string            `json:"external_ref"`
	Seq         int64             `json:"seq"`
	OccurredAt  time.Time         `json:"occurred_at"`
	Payload     string            `json:"payload"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// ApplyStatus reports what applying one event did.
type ApplyStatus string

// Stream enrichment apply outcomes.
const (
	StatusApplied ApplyStatus = "applied" // merged into the user row
	StatusDeduped ApplyStatus = "deduped" // event was already applied
	StatusStale   ApplyStatus = "stale"   // a newer seq was already applied
)

// ApplyResult pairs the outcome with the user row it targeted.
type ApplyResult struct {
	Status ApplyStatus `json:"status"`
	UserID string      `json:"user_id"`
}

// OutboxEvent is a domain event written in the same database transaction as
// the change it describes; the relay publishes it to the broker fabric
// afterwards. Headers propagate trace context across the relay hop.
type OutboxEvent struct {
	ID          string            `json:"id"`
	EventType   string            `json:"event_type"`
	AggregateID string            `json:"aggregate_id"`
	Payload     string            `json:"payload"`
	Headers     map[string]string `json:"headers,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	ClaimedAt   *time.Time        `json:"claimed_at,omitempty"`
	ProcessedAt *time.Time        `json:"processed_at"`
	Attempts    int               `json:"attempts"`
}

// OutboxUserEnriched is the event type published when a user row changes
// through the enrichment streams.
const OutboxUserEnriched = "user.enriched"

// DeadLetterReason classifies why a message was parked.
const (
	ReasonMalformed   = "malformed"    // payload failed decode/validation
	ReasonUnknownUser = "unknown_user" // no user carries the external_ref
	ReasonMaxAttempts = "max_attempts" // relay exhausted its attempts
)

// DeadLetter is a poison or unprocessable message parked for inspection and
// replay instead of being dropped or blocking its stream forever.
type DeadLetter struct {
	ID         string     `json:"id"`
	Source     string     `json:"source"`
	Topic      string     `json:"topic"`
	Payload    string     `json:"payload"`
	Reason     string     `json:"reason"`
	Attempts   int        `json:"attempts"`
	CreatedAt  time.Time  `json:"created_at"`
	ReplayedAt *time.Time `json:"replayed_at"`
}
