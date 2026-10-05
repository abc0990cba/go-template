package postgres

// The email delivery log, in PostgreSQL: one row per outbound message, the
// delivery lifecycle (queued → sent/failed → dead), and the retention sweep.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/postgres/db"
)

var _ email.MessageStore = (*Store)(nil)

// CreateEmailMessage records one outbound message.
func (s *Store) CreateEmailMessage(ctx context.Context, msg email.Message) error {
	return mapErr(s.q.CreateEmailMessage(ctx, db.CreateEmailMessageParams{
		ID: msg.ID, ToAddress: msg.To, Subject: msg.Subject, Body: msg.Body,
		Kind: msg.Kind, Status: msg.Status, Attempts: clampInt32(msg.Attempts),
		LastError: msg.LastError, CreatedAt: msg.CreatedAt,
	}))
}

// GetEmailMessage returns one recorded message.
func (s *Store) GetEmailMessage(ctx context.Context, id string) (email.Message, error) {
	row, err := s.q.GetEmailMessage(ctx, id)
	if err != nil {
		return email.Message{}, mapErr(err)
	}
	return emailMessageFromRow(row.ID, row.ToAddress, row.Subject, row.Body, row.Kind,
		row.Status, row.Attempts, row.LastError, row.CreatedAt, row.SentAt), nil
}

// ListEmailMessages returns the log newest-first, optionally filtered.
func (s *Store) ListEmailMessages(ctx context.Context, status string, limit, offset int) ([]email.Message, error) {
	rows, err := s.q.ListEmailMessages(ctx, db.ListEmailMessagesParams{
		Column1: status, Limit: clampInt32(limit), Offset: clampInt32(offset),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]email.Message, 0, len(rows))
	for _, row := range rows {
		out = append(out, emailMessageFromRow(row.ID, row.ToAddress, row.Subject, row.Body, row.Kind,
			row.Status, row.Attempts, row.LastError, row.CreatedAt, row.SentAt))
	}
	return out, nil
}

// UpdateEmailMessageStatus records one delivery attempt's outcome.
func (s *Store) UpdateEmailMessageStatus(ctx context.Context, id, status string, attempts int, lastError string, sentAt *time.Time) error {
	var sent pgtype.Timestamptz
	if sentAt != nil {
		sent = pgtype.Timestamptz{Time: *sentAt, Valid: true}
	}
	n, err := s.q.UpdateEmailMessageStatus(ctx, db.UpdateEmailMessageStatusParams{
		ID: id, Status: status, Attempts: clampInt32(attempts), LastError: lastError, SentAt: sent,
	})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return kernel.ErrNotFound
	}
	return nil
}

// PruneEmailMessages deletes rows older than the cutoff.
func (s *Store) PruneEmailMessages(ctx context.Context, olderThan time.Time) (int64, error) {
	return s.q.PruneEmailMessages(ctx, olderThan)
}

// clampInt32 pins list and counter values into the int32 range the
// columns hold; callers pass bounded limits and small counters.
func clampInt32(v int) int32 {
	if v < 0 {
		return 0
	}
	if v > 1<<31-1 {
		return 1<<31 - 1
	}
	return int32(v) // #nosec G115 -- clamped above
}

func emailMessageFromRow(id, to, subject, body, kind, status string, attempts int32,
	lastError string, createdAt time.Time, sentAt pgtype.Timestamptz) email.Message {
	return email.Message{
		ID: id, To: to, Subject: subject, Body: body, Kind: kind,
		Status: status, Attempts: int(attempts), LastError: lastError,
		CreatedAt: createdAt, SentAt: timeFromPtr(sentAt),
	}
}
