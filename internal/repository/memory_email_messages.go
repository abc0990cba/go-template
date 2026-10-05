package repository

// The email delivery log, in memory. Mirrors the postgres adapter: one row
// per outbound message, newest first, filterable by status.

import (
	"context"
	"sort"
	"time"

	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/kernel"
)

// CreateEmailMessage records one outbound message.
func (s *MemoryStore) CreateEmailMessage(_ context.Context, msg email.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	s.emailMessages = append(s.emailMessages, msg)
	return nil
}

// GetEmailMessage returns one recorded message.
func (s *MemoryStore) GetEmailMessage(_ context.Context, id string) (email.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, msg := range s.emailMessages {
		if msg.ID == id {
			return msg, nil
		}
	}
	return email.Message{}, kernel.ErrNotFound
}

// ListEmailMessages returns the log newest-first, optionally filtered.
func (s *MemoryStore) ListEmailMessages(_ context.Context, status string, limit, offset int) ([]email.Message, error) {
	s.mu.RLock()
	rows := make([]email.Message, 0, len(s.emailMessages))
	for _, msg := range s.emailMessages {
		if status == "" || msg.Status == status {
			rows = append(rows, msg)
		}
	}
	s.mu.RUnlock()

	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	if offset > len(rows) {
		offset = len(rows)
	}
	rows = rows[offset:]
	if limit <= 0 || limit > len(rows) {
		limit = len(rows)
	}
	return rows[:limit], nil
}

// UpdateEmailMessageStatus records one delivery attempt's outcome.
func (s *MemoryStore) UpdateEmailMessageStatus(_ context.Context, id, status string, attempts int, lastError string, sentAt *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.emailMessages {
		if s.emailMessages[i].ID == id {
			s.emailMessages[i].Status = status
			s.emailMessages[i].Attempts = attempts
			s.emailMessages[i].LastError = lastError
			s.emailMessages[i].SentAt = sentAt
			return nil
		}
	}
	return kernel.ErrNotFound
}

// PruneEmailMessages deletes rows older than the cutoff.
func (s *MemoryStore) PruneEmailMessages(_ context.Context, olderThan time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.emailMessages[:0]
	var pruned int64
	for _, msg := range s.emailMessages {
		if msg.CreatedAt.Before(olderThan) {
			pruned++
			continue
		}
		kept = append(kept, msg)
	}
	s.emailMessages = kept
	return pruned, nil
}
