package repository

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/streams"
)

// SaveDeadLetter parks an unprocessable message; duplicate ids are ignored.
func (s *MemoryStore) SaveDeadLetter(_ context.Context, letter streams.DeadLetter) error {
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
func (s *MemoryStore) GetDeadLetter(_ context.Context, id string) (streams.DeadLetter, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, letter := range s.deadLetters {
		if letter.ID == id {
			return letter, nil
		}
	}
	return streams.DeadLetter{}, kernel.ErrNotFound
}

// ListDeadLetters pages parked messages newest first; opts.Pending filters to
// not-yet-replayed ones.
func (s *MemoryStore) ListDeadLetters(_ context.Context, opts kernel.DeadLetterOptions) ([]streams.DeadLetter, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := make([]streams.DeadLetter, len(s.deadLetters))
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
		return []streams.DeadLetter{}, "", nil
	}
	end := min(start+limit, len(filtered))
	next := ""
	if end < len(filtered) {
		next = filtered[end-1].ID
	}
	return filtered[start:end], next, nil
}

// MarkDeadLetterReplayed stamps the replay time; kernel.ErrNotFound when absent or
// already replayed.
func (s *MemoryStore) MarkDeadLetterReplayed(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, letter := range s.deadLetters {
		if letter.ID == id {
			if letter.ReplayedAt != nil {
				return kernel.ErrNotFound
			}
			stamp := time.Now()
			s.deadLetters[i].ReplayedAt = &stamp
			return nil
		}
	}
	return kernel.ErrNotFound
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
