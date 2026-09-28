package postgres

// Streams on PostgreSQL: exactly-once enrichment application, the
// transactional outbox, and dead letters. Every apply runs in one
// transaction — row lock, inbox dedupe, sequence guard, owned-fields merge,
// outbox write — so a crash can never leave the user row and its emitted
// event out of sync.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/platform/postgres/db"
	"github.com/your-org/go-template/internal/repository"
)

// ApplyStreamEvent merges one enrichment event into its user row with
// exactly-once semantics per (source, event_id):
//
//  1. The user row is locked by external_ref — every source for this user
//     serializes here, in one place, so no two transactions race.
//  2. The inbox insert dedupes redeliveries (unique (source, event_id)).
//  3. The update carries a per-source sequence guard: stale events are
//     recorded as processed without touching the row.
//  4. The JSONB merge only touches the fields the source owns; a
//     concurrent event for the other source never conflicts.
//  5. The outbox row commits with the merge or not at all.
func (s *Store) ApplyStreamEvent(ctx context.Context, event model.StreamEvent) (model.StreamApplyResult, error) {
	var result model.StreamApplyResult
	err := s.tx(ctx, func(q *db.Queries) error {
		lock, err := q.LockUserByExternalRefForSeq(ctx, textOrNull(event.ExternalRef))
		if err != nil {
			return mapErr(err) // ErrNotFound when no user carries the ref yet
		}
		userID := lock.ID

		inserted, err := q.InsertProcessedEvent(ctx, db.InsertProcessedEventParams{
			Source: event.Source, EventID: event.EventID, UserID: userID, Seq: event.Seq,
		})
		if err != nil {
			return mapErr(err)
		}
		if inserted == 0 {
			result = model.StreamApplyResult{Status: model.StreamDeduped, UserID: userID}
			return nil
		}

		var applied int64
		switch event.Source {
		case model.SourceKafka:
			applied, err = q.ApplyUserProfile(ctx, db.ApplyUserProfileParams{
				ID: userID, Column2: event.Payload, EnrichProfileSeq: event.Seq,
			})
		case model.SourceNats:
			applied, err = q.ApplyUserContact(ctx, db.ApplyUserContactParams{
				ID: userID, Column2: event.Payload, EnrichContactSeq: event.Seq,
			})
		default:
			return fmt.Errorf("unknown stream source %q", event.Source)
		}
		if err != nil {
			return mapErr(err)
		}
		if applied == 0 {
			result = model.StreamApplyResult{Status: model.StreamStale, UserID: userID}
			return nil
		}

		payload, err := json.Marshal(map[string]any{
			"user_id": userID, "external_ref": event.ExternalRef,
			"source": event.Source, "seq": event.Seq,
		})
		if err != nil {
			return fmt.Errorf("outbox payload: %w", err)
		}
		headers, err := json.Marshal(event.Headers)
		if err != nil {
			return fmt.Errorf("outbox headers: %w", err)
		}
		if string(headers) == "null" {
			headers = []byte("{}")
		}
		if err := q.InsertOutboxEvent(ctx, db.InsertOutboxEventParams{
			ID: uuid.NewString(), EventType: model.OutboxUserEnriched, AggregateID: userID,
			Column4: string(payload), Column5: string(headers),
		}); err != nil {
			return mapErr(err)
		}
		result = model.StreamApplyResult{Status: model.StreamApplied, UserID: userID}
		return nil
	})
	return result, err
}

// StreamSeqs returns the applied per-source sequence numbers of a user.
func (s *Store) StreamSeqs(ctx context.Context, userID string) (profile, contact int64, err error) {
	row, err := s.q.GetUserEnrichSeqs(ctx, userID)
	if err != nil {
		return 0, 0, mapErr(err)
	}
	return row.EnrichProfileSeq, row.EnrichContactSeq, nil
}

// ClaimOutboxBatch leases up to limit unprocessed events whose previous
// lease (if any) has expired. SKIP LOCKED keeps concurrent relays from
// double-claiming.
func (s *Store) ClaimOutboxBatch(ctx context.Context, limit, leaseSeconds, maxAttempts int) ([]model.OutboxEvent, error) {
	rows, err := s.q.ClaimOutboxBatch(ctx, db.ClaimOutboxBatchParams{
		LeaseSecs:   int32(leaseSeconds), // #nosec G115 -- caller clamps
		MaxAttempts: int32(maxAttempts),  // #nosec G115 -- caller clamps
		Lim:         int32(limit),        // #nosec G115 -- caller clamps
	})
	if err != nil {
		return nil, mapErr(err)
	}
	events := make([]model.OutboxEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, outboxFromRow(row))
	}
	return events, nil
}

// MarkOutboxProcessed completes published events.
func (s *Store) MarkOutboxProcessed(ctx context.Context, ids []string) error {
	_, err := s.q.MarkOutboxProcessed(ctx, ids)
	return mapErr(err)
}

// ReleaseOutboxBatch returns failed events to the pool and counts the
// attempt; the stale lease filter re-claims them after the cool-down.
func (s *Store) ReleaseOutboxBatch(ctx context.Context, ids []string) error {
	return mapErr(s.q.ReleaseOutboxBatch(ctx, ids))
}

// FailOutboxPermanently marks exhausted events processed (dead-lettered by
// the relay beforehand).
func (s *Store) FailOutboxPermanently(ctx context.Context, ids []string) error {
	return mapErr(s.q.FailOutboxPermanently(ctx, ids))
}

// CountOutboxPending reports events awaiting publication.
func (s *Store) CountOutboxPending(ctx context.Context) (int, error) {
	n, err := s.q.CountOutboxPending(ctx)
	return int(n), mapErr(err)
}

// ListOutbox pages events newest first; opts.Status "pending" filters to
// unpublished rows.
func (s *Store) ListOutbox(ctx context.Context, opts repository.ListOptions) ([]model.OutboxEvent, string, error) {
	limit := int32(25)
	if opts.Limit > 0 && opts.Limit <= 100 {
		limit = int32(opts.Limit)
	}
	pending := opts.Status == "pending"
	rows, err := s.q.ListOutboxOffset(ctx, db.ListOutboxOffsetParams{
		Pending: pending,
		Off:     int32(max(opts.Offset, 0)), // #nosec G115 -- clamped
		Lim:     limit + 1,
	})
	if err != nil {
		return nil, "", mapErr(err)
	}
	next := ""
	if len(rows) > int(limit) {
		rows = rows[:limit]
		next = rows[limit-1].ID
	}
	events := make([]model.OutboxEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, outboxFromRow(row))
	}
	return events, next, nil
}

func outboxFromRow(row db.Outbox) model.OutboxEvent {
	var headers map[string]string
	if row.Headers != "" {
		_ = json.Unmarshal([]byte(row.Headers), &headers)
	}
	return model.OutboxEvent{
		ID: row.ID, EventType: row.EventType, AggregateID: row.AggregateID,
		Payload: row.Payload, Headers: headers,
		CreatedAt: row.CreatedAt, ClaimedAt: timeFromPtr(row.ClaimedAt),
		ProcessedAt: timeFromPtr(row.ProcessedAt),
		Attempts:    int(row.Attempts),
	}
}

// SaveDeadLetter parks an unprocessable message; duplicate ids are ignored so
// redeliveries cannot stack copies.
func (s *Store) SaveDeadLetter(ctx context.Context, letter model.DeadLetter) error {
	if letter.ID == "" {
		letter.ID = uuid.NewString()
	}
	return mapErr(s.q.InsertDeadLetter(ctx, db.InsertDeadLetterParams{
		ID: letter.ID, Source: letter.Source, Topic: letter.Topic,
		Payload: letter.Payload, Reason: letter.Reason, Attempts: int32(letter.Attempts), // #nosec G115 -- set by caller
	}))
}

// GetDeadLetter returns one parked message.
func (s *Store) GetDeadLetter(ctx context.Context, id string) (model.DeadLetter, error) {
	row, err := s.q.GetDeadLetter(ctx, id)
	if err != nil {
		return model.DeadLetter{}, mapErr(err)
	}
	return deadLetterFromRow(row), nil
}

// ListDeadLetters pages parked messages newest first; opts.Pending filters to
// not-yet-replayed ones.
func (s *Store) ListDeadLetters(ctx context.Context, opts repository.DeadLetterOptions) ([]model.DeadLetter, string, error) {
	limit := int32(25)
	if opts.Limit > 0 && opts.Limit <= 100 {
		limit = int32(opts.Limit)
	}
	rows, err := s.q.ListDeadLettersOffset(ctx, db.ListDeadLettersOffsetParams{
		Pending: opts.Pending,
		Off:     int32(max(opts.Offset, 0)), // #nosec G115 -- clamped
		Lim:     limit + 1,
	})
	if err != nil {
		return nil, "", mapErr(err)
	}
	next := ""
	if len(rows) > int(limit) {
		rows = rows[:limit]
		next = rows[limit-1].ID
	}
	letters := make([]model.DeadLetter, 0, len(rows))
	for _, row := range rows {
		letters = append(letters, deadLetterFromRow(row))
	}
	return letters, next, nil
}

// MarkDeadLetterReplayed stamps the replay time; it reports ErrNotFound when
// the letter is absent or already replayed.
func (s *Store) MarkDeadLetterReplayed(ctx context.Context, id string) error {
	n, err := s.q.MarkDeadLetterReplayed(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// CountDeadLettersPending reports parked messages awaiting replay.
func (s *Store) CountDeadLettersPending(ctx context.Context) (int, error) {
	n, err := s.q.CountDeadLettersPending(ctx)
	return int(n), mapErr(err)
}

func deadLetterFromRow(row db.DeadLetter) model.DeadLetter {
	return model.DeadLetter{
		ID: row.ID, Source: row.Source, Topic: row.Topic, Payload: row.Payload,
		Reason: row.Reason, Attempts: int(row.Attempts),
		CreatedAt: row.CreatedAt, ReplayedAt: timeFromPtr(row.ReplayedAt),
	}
}

// ReserveIdempotency inserts the reservation atomically. When the (user, key)
// pair already exists it returns the stored record with already=true.
func (s *Store) ReserveIdempotency(ctx context.Context, record model.IdempotencyRecord) (model.IdempotencyRecord, bool, error) {
	row, err := s.q.ReserveIdempotencyKey(ctx, db.ReserveIdempotencyKeyParams{
		Key: record.Key, UserID: record.UserID, Method: record.Method, Path: record.Path,
		RequestHash: record.RequestHash, ExpiresAt: record.ExpiresAt,
	})
	if err == nil {
		return idempotencyFromRow(row), false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.IdempotencyRecord{}, false, mapErr(err)
	}
	// Conflict: someone reserved this (user, key) first; read their record.
	existing, err := s.q.GetIdempotencyKey(ctx, db.GetIdempotencyKeyParams{Key: record.Key, UserID: record.UserID})
	if err != nil {
		return model.IdempotencyRecord{}, false, mapErr(err)
	}
	return idempotencyFromRow(existing), true, nil
}

// CompleteIdempotency stores the response of the request that owned the
// reservation; only the in-flight owner can write it.
func (s *Store) CompleteIdempotency(ctx context.Context, key, userID string, status int, body []byte, contentType string) error {
	return mapErr(s.q.CompleteIdempotencyKey(ctx, db.CompleteIdempotencyKeyParams{
		Key: key, UserID: userID, Status: int32(status), ResponseBody: body, ContentType: contentType, // #nosec G115 -- HTTP status fits int32
	}))
}

// PruneIdempotency deletes expired reservations and responses.
func (s *Store) PruneIdempotency(ctx context.Context) (int64, error) {
	return s.q.PruneIdempotencyKeys(ctx)
}

func idempotencyFromRow(row db.IdempotencyKey) model.IdempotencyRecord {
	return model.IdempotencyRecord{
		Key: row.Key, UserID: row.UserID, Method: row.Method, Path: row.Path,
		RequestHash: row.RequestHash, Status: int(row.Status),
		ResponseBody: row.ResponseBody, ContentType: row.ContentType,
		CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt,
	}
}
