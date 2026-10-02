package repository

import (
	"context"
	"sort"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/tickets"
)

// --- Ticket types -------------------------------------------------------

// CreateTicketType stores a ticket type; (org_id, key) is unique.
func (s *MemoryStore) CreateTicketType(_ context.Context, typ tickets.TicketType) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.ticketTypes {
		if existing.OrgID == typ.OrgID && existing.Key == typ.Key {
			return kernel.ErrConflict
		}
	}
	s.ticketTypes[typ.ID] = typ
	return nil
}

// GetTicketType returns the type by id.
func (s *MemoryStore) GetTicketType(_ context.Context, id string) (tickets.TicketType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	typ, ok := s.ticketTypes[id]
	if !ok {
		return tickets.TicketType{}, kernel.ErrNotFound
	}
	return typ, nil
}

// GetTicketTypeByKey resolves a type in one organization's namespace.
func (s *MemoryStore) GetTicketTypeByKey(_ context.Context, orgID, key string) (tickets.TicketType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, typ := range s.ticketTypes {
		if typ.OrgID == orgID && typ.Key == key {
			return typ, nil
		}
	}
	return tickets.TicketType{}, kernel.ErrNotFound
}

// UpdateTicketType overwrites the type.
func (s *MemoryStore) UpdateTicketType(_ context.Context, typ tickets.TicketType) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ticketTypes[typ.ID]; !ok {
		return kernel.ErrNotFound
	}
	s.ticketTypes[typ.ID] = typ
	return nil
}

// DeleteTicketType removes the type by id.
func (s *MemoryStore) DeleteTicketType(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ticketTypes[id]; !ok {
		return kernel.ErrNotFound
	}
	delete(s.ticketTypes, id)
	return nil
}

// ListTicketTypes returns the platform-wide types plus the organization's
// own, ordered by key.
func (s *MemoryStore) ListTicketTypes(_ context.Context, orgID string) ([]tickets.TicketType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]tickets.TicketType, 0, len(s.ticketTypes))
	for _, typ := range s.ticketTypes {
		if typ.OrgID == "" || typ.OrgID == orgID {
			out = append(out, typ)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// --- Comments and timeline ----------------------------------------------

// AddTicketComment appends one comment.
func (s *MemoryStore) AddTicketComment(_ context.Context, comment tickets.TicketComment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if comment.CreatedAt.IsZero() {
		comment.CreatedAt = time.Now()
	}
	s.comments = append(s.comments, comment)
	return nil
}

// ListTicketComments pages the ticket's comments newest first.
func (s *MemoryStore) ListTicketComments(_ context.Context, ticketID string, limit, offset int) ([]tickets.TicketComment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rows []tickets.TicketComment
	for _, comment := range s.comments {
		if comment.TicketID == ticketID {
			rows = append(rows, comment)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	start := max(offset, 0)
	if start >= len(rows) {
		return nil, nil
	}
	end := min(start+limit, len(rows))
	return rows[start:end], nil
}

// AddTicketEvent appends one timeline entry.
func (s *MemoryStore) AddTicketEvent(_ context.Context, event tickets.TicketEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	s.ticketEvents = append(s.ticketEvents, event)
	return nil
}

// ListTicketEvents pages the ticket's timeline newest first.
func (s *MemoryStore) ListTicketEvents(_ context.Context, ticketID string, limit, offset int) ([]tickets.TicketEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rows []tickets.TicketEvent
	for _, event := range s.ticketEvents {
		if event.TicketID == ticketID {
			rows = append(rows, event)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	start := max(offset, 0)
	if start >= len(rows) {
		return nil, nil
	}
	end := min(start+limit, len(rows))
	return rows[start:end], nil
}
