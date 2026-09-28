package repository

// Memory-store implementation of the stream ports. It mirrors the SQL
// transaction under the store mutex: dedupe by (source, event_id), a
// per-source sequence guard, JSONB-style field merge into the owned user
// column, and the outbox row written with the merge or not at all.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/your-org/go-template/internal/model"
)

// streamEventKey builds the dedupe map key.
func streamEventKey(source, eventID string) string {
	return source + "\x00" + eventID
}

// idempotencyKey builds the reservation map key.
func idempotencyKey(userID, key string) string {
	return userID + "\x00" + key
}

// ApplyStreamEvent applies one event with exactly-once semantics, locking
// the whole sequence under the store mutex exactly as the SQL version locks
// the user row.
func (s *MemoryStore) ApplyStreamEvent(_ context.Context, event model.StreamEvent) (model.StreamApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var owner string
	found := false
	for id, user := range s.users {
		if user.ExternalRef == event.ExternalRef {
			owner, found = id, true
			break
		}
	}
	if !found {
		return model.StreamApplyResult{}, ErrNotFound
	}

	key := streamEventKey(event.Source, event.EventID)
	if _, done := s.processedEvents[key]; done {
		return model.StreamApplyResult{Status: model.StreamDeduped, UserID: owner}, nil
	}

	var applied bool
	switch event.Source {
	case model.SourceKafka:
		if event.Seq > s.enrichSeqs[owner].profile {
			user := s.users[owner]
			user.EnrichProfile = mergeJSON(user.EnrichProfile, event.Payload)
			user.EnrichedAt = ptr(time.Now())
			user.UpdatedAt = time.Now()
			s.users[owner] = user
			seqs := s.enrichSeqs[owner]
			seqs.profile = event.Seq
			s.enrichSeqs[owner] = seqs
			applied = true
		}
	case model.SourceNats:
		if event.Seq > s.enrichSeqs[owner].contact {
			user := s.users[owner]
			user.EnrichContact = mergeJSON(user.EnrichContact, event.Payload)
			user.EnrichedAt = ptr(time.Now())
			user.UpdatedAt = time.Now()
			s.users[owner] = user
			seqs := s.enrichSeqs[owner]
			seqs.contact = event.Seq
			s.enrichSeqs[owner] = seqs
			applied = true
		}
	default:
		return model.StreamApplyResult{}, fmt.Errorf("unknown stream source %q", event.Source)
	}
	s.processedEvents[key] = model.StreamApplyResult{UserID: owner}
	if !applied {
		return model.StreamApplyResult{Status: model.StreamStale, UserID: owner}, nil
	}

	payload, err := json.Marshal(map[string]any{
		"user_id": owner, "external_ref": event.ExternalRef,
		"source": event.Source, "seq": event.Seq,
	})
	if err != nil {
		return model.StreamApplyResult{}, err
	}
	s.outboxItems = append(s.outboxItems, &model.OutboxEvent{
		ID:          uuid.NewString(),
		EventType:   model.OutboxUserEnriched,
		AggregateID: owner,
		Payload:     string(payload),
		Headers:     event.Headers,
		CreatedAt:   time.Now(),
	})
	return model.StreamApplyResult{Status: model.StreamApplied, UserID: owner}, nil
}

// mergeJSON overlays the incoming JSON object onto the stored one, field by
// field — the in-memory twin of the SQL `stored || incoming` operator.
func mergeJSON(stored, incoming string) string {
	var patch map[string]any
	if err := json.Unmarshal([]byte(incoming), &patch); err != nil || patch == nil {
		return stored
	}
	var base map[string]any
	_ = json.Unmarshal([]byte(stored), &base)
	if base == nil {
		base = map[string]any{}
	}
	for k, v := range patch {
		base[k] = v
	}
	out, err := json.Marshal(base)
	if err != nil {
		return stored
	}
	return string(out)
}

func ptr[T any](v T) *T { return &v }

// StreamSeqs returns the applied per-source sequence numbers of a user.
func (s *MemoryStore) StreamSeqs(_ context.Context, userID string) (profile, contact int64, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return 0, 0, ErrNotFound
	}
	seqs := s.enrichSeqs[userID]
	return seqs.profile, seqs.contact, nil
}

// ClaimOutboxBatch leases unprocessed events whose lease expired.
func (s *MemoryStore) ClaimOutboxBatch(_ context.Context, limit, leaseSeconds, maxAttempts int) ([]model.OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	lease := time.Duration(leaseSeconds) * time.Second
	var claimed []model.OutboxEvent
	for _, item := range s.outboxItems {
		if len(claimed) >= limit {
			break
		}
		if item.ProcessedAt != nil || item.Attempts >= maxAttempts {
			continue
		}
		if item.ClaimedAt != nil && now.Sub(*item.ClaimedAt) < lease {
			continue
		}
		stamp := now
		item.ClaimedAt = &stamp
		claimed = append(claimed, *item)
	}
	return claimed, nil
}

// MarkOutboxProcessed completes published events.
func (s *MemoryStore) MarkOutboxProcessed(_ context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, item := range s.outboxItems {
		for _, id := range ids {
			if item.ID == id {
				stamp := now
				item.ProcessedAt = &stamp
				item.ClaimedAt = nil
			}
		}
	}
	return nil
}

// ReleaseOutboxBatch returns failed events to the pool with an attempts bump.
func (s *MemoryStore) ReleaseOutboxBatch(_ context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.outboxItems {
		for _, id := range ids {
			if item.ID == id {
				item.ClaimedAt = nil
				item.Attempts++
			}
		}
	}
	return nil
}

// FailOutboxPermanently marks exhausted events processed.
func (s *MemoryStore) FailOutboxPermanently(ctx context.Context, ids []string) error {
	return s.MarkOutboxProcessed(ctx, ids)
}

// CountOutboxPending reports events awaiting publication.
func (s *MemoryStore) CountOutboxPending(_ context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, item := range s.outboxItems {
		if item.ProcessedAt == nil {
			n++
		}
	}
	return n, nil
}

// ListOutbox pages events newest first; opts.Status "pending" filters to
// unpublished rows.
func (s *MemoryStore) ListOutbox(_ context.Context, opts ListOptions) ([]model.OutboxEvent, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := make([]model.OutboxEvent, 0, len(s.outboxItems))
	for _, item := range s.outboxItems {
		if opts.Status == "pending" && item.ProcessedAt != nil {
			continue
		}
		rows = append(rows, *item)
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.After(rows[j].CreatedAt)
		}
		return rows[i].ID > rows[j].ID
	})
	limit := 25
	if opts.Limit > 0 && opts.Limit <= 100 {
		limit = opts.Limit
	}
	start := max(opts.Offset, 0)
	if start >= len(rows) {
		return []model.OutboxEvent{}, "", nil
	}
	end := min(start+limit, len(rows))
	next := ""
	if end < len(rows) {
		next = rows[end-1].ID
	}
	return rows[start:end], next, nil
}

// SaveDeadLetter parks an unprocessable message; duplicate ids are ignored.
func (s *MemoryStore) SaveDeadLetter(_ context.Context, letter model.DeadLetter) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if letter.ID == "" {
		letter.ID = uuid.NewString()
	}
	for _, existing := range s.deadLetters {
		if existing.ID == letter.ID {
			return nil
		}
	}
	letter.CreatedAt = time.Now()
	s.deadLetters = append(s.deadLetters, letter)
	return nil
}

// GetDeadLetter returns one parked message.
func (s *MemoryStore) GetDeadLetter(_ context.Context, id string) (model.DeadLetter, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, letter := range s.deadLetters {
		if letter.ID == id {
			return letter, nil
		}
	}
	return model.DeadLetter{}, ErrNotFound
}

// ListDeadLetters pages parked messages newest first; opts.Pending filters to
// not-yet-replayed ones.
func (s *MemoryStore) ListDeadLetters(_ context.Context, opts DeadLetterOptions) ([]model.DeadLetter, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := make([]model.DeadLetter, len(s.deadLetters))
	copy(rows, s.deadLetters)
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.After(rows[j].CreatedAt)
		}
		return rows[i].ID > rows[j].ID
	})
	filtered := rows[:0]
	for _, letter := range rows {
		if opts.Pending && letter.ReplayedAt != nil {
			continue
		}
		filtered = append(filtered, letter)
	}
	limit := 25
	if opts.Limit > 0 && opts.Limit <= 100 {
		limit = opts.Limit
	}
	start := max(opts.Offset, 0)
	if start >= len(filtered) {
		return []model.DeadLetter{}, "", nil
	}
	end := min(start+limit, len(filtered))
	next := ""
	if end < len(filtered) {
		next = filtered[end-1].ID
	}
	return filtered[start:end], next, nil
}

// MarkDeadLetterReplayed stamps the replay time; ErrNotFound when absent or
// already replayed.
func (s *MemoryStore) MarkDeadLetterReplayed(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, letter := range s.deadLetters {
		if letter.ID == id {
			if letter.ReplayedAt != nil {
				return ErrNotFound
			}
			stamp := time.Now()
			s.deadLetters[i].ReplayedAt = &stamp
			return nil
		}
	}
	return ErrNotFound
}

// CountDeadLettersPending reports parked messages awaiting replay.
func (s *MemoryStore) CountDeadLettersPending(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, letter := range s.deadLetters {
		if letter.ReplayedAt == nil {
			n++
		}
	}
	return n, nil
}

// ReserveIdempotency inserts the reservation atomically; already=true when
// the (user, key) pair exists.
func (s *MemoryStore) ReserveIdempotency(ctx context.Context, record model.IdempotencyRecord) (model.IdempotencyRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := idempotencyKey(record.UserID, record.Key)
	if existing, ok := s.idempotency[key]; ok {
		return existing, true, nil
	}
	record.CreatedAt = time.Now()
	s.idempotency[key] = record
	return record, false, nil
}

// CompleteIdempotency stores the response of the request owning the
// reservation.
func (s *MemoryStore) CompleteIdempotency(ctx context.Context, key, userID string, status int, body []byte, contentType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	mapKey := idempotencyKey(userID, key)
	record, ok := s.idempotency[mapKey]
	if !ok || record.Status != 0 {
		return nil
	}
	record.Status = status
	record.ResponseBody = body
	record.ContentType = contentType
	s.idempotency[mapKey] = record
	return nil
}

// PruneIdempotency deletes expired reservations and responses.
func (s *MemoryStore) PruneIdempotency(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var pruned int64
	for key, record := range s.idempotency {
		if now.After(record.ExpiresAt) {
			delete(s.idempotency, key)
			pruned++
		}
	}
	return pruned, nil
}
