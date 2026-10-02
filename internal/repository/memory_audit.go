package repository

import (
	"context"
	"time"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/kernel"
)

// RecordAudit appends one immutable audit record.
func (s *MemoryStore) RecordAudit(_ context.Context, entry audit.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	s.auditLog = append(s.auditLog, entry)
	return nil
}

// ListAudit returns the trail newest first, filtered by the options. The
// fake keeps every record; retention is a PostgreSQL concern.
func (s *MemoryStore) ListAudit(_ context.Context, opts kernel.AuditOptions) ([]audit.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := make([]audit.Entry, 0, len(s.auditLog))
	for _, entry := range s.auditLog {
		if opts.Subject != "" {
			isActor := entry.ActorID == opts.Subject
			isSubject := entry.ResourceType == "user" && entry.ResourceID == opts.Subject
			if !isActor && !isSubject {
				continue
			}
		}
		if opts.Actor != "" && entry.ActorID != opts.Actor {
			continue
		}
		if opts.Action != "" && entry.Action != opts.Action {
			continue
		}
		if opts.Resource != "" && entry.ResourceType != opts.Resource {
			continue
		}
		if opts.ResourceID != "" && entry.ResourceID != opts.ResourceID {
			continue
		}
		rows = append(rows, entry)
	}
	// Newest first without reordering the append-only slice.
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	start := max(opts.Offset, 0)
	if start >= len(rows) {
		return nil, nil
	}
	end := min(start+limit, len(rows))
	return rows[start:end], nil
}
