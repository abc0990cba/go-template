// Package audit owns the immutable audit trail: who did what, when, from
// where, with before/after diffs. Records are never updated or deleted
// except by retention.
package audit

import (
	"context"
	"log/slog"
	"time"

	"github.com/your-org/go-template/internal/kernel"
)

// Entry is one immutable audit record: an actor performed action on a
// resource in a domain; Diff carries the before/after JSON snapshot, IP and
// RequestID tie the record to the originating request.
type Entry struct {
	ID           string    `json:"id"`
	ActorID      string    `json:"actor_id"`
	ActorEmail   string    `json:"actor_email"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id,omitempty"`
	Domain       string    `json:"domain,omitempty"`
	Diff         string    `json:"diff,omitempty"`
	IP           string    `json:"ip,omitempty"`
	RequestID    string    `json:"request_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Store persists the append-only trail.
type Store interface {
	RecordAudit(ctx context.Context, entry Entry) error
	ListAudit(ctx context.Context, opts kernel.AuditOptions) ([]Entry, error)
}

// Service appends immutable records to the audit trail. Handlers call
// Record (the HTTP helper) so actor, domain, ip, and request id are
// captured consistently; write failures degrade to log noise, never to a
// failed request.
type Service struct {
	store  Store
	logger *slog.Logger
}

// NewService builds the service; a nil store discards records (dev mode
// without persistence).
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Record writes one entry, stamping the timestamp. The caller supplies the
// rest of the context.
func (s *Service) Record(ctx context.Context, entry Entry) {
	if s == nil || s.store == nil {
		return
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	if entry.ID == "" {
		entry.ID = kernel.NewID()
	}
	if err := s.store.RecordAudit(ctx, entry); err != nil {
		if s.logger != nil {
			s.logger.Error("audit record failed", "action", entry.Action, "err", err)
		}
	}
}

// List pages the trail newest first, filtered by the options.
func (s *Service) List(ctx context.Context, opts kernel.AuditOptions) ([]Entry, error) {
	if s.store == nil {
		return nil, nil
	}
	return s.store.ListAudit(ctx, opts)
}
