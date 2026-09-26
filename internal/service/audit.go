package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
)

// AuditService appends immutable records to the audit trail. Handlers call
// Record through the HTTP helper so actor, domain, ip, and request id are
// captured consistently; write failures degrade to log noise, never to a
// failed request.
type AuditService struct {
	store  AuditRepo
	logger *slog.Logger
}

// NewAuditService builds the service; a nil store discards records (dev
// mode without persistence).
func NewAuditService(store AuditRepo) *AuditService {
	return &AuditService{store: store}
}

// Record writes one entry, stamping the timestamp. The caller supplies the
// rest of the context.
func (s *AuditService) Record(ctx context.Context, entry model.AuditEntry) {
	if s.store == nil {
		return
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	if entry.ID == "" {
		entry.ID = newID()
	}
	if err := s.store.RecordAudit(ctx, entry); err != nil {
		if s.logger != nil {
			s.logger.Error("audit record failed", "action", entry.Action, "err", err)
		}
	}
}

// List pages the trail newest first, filtered by the options.
func (s *AuditService) List(ctx context.Context, opts repository.AuditOptions) ([]model.AuditEntry, error) {
	if s.store == nil {
		return nil, nil
	}
	return s.store.ListAudit(ctx, opts)
}
