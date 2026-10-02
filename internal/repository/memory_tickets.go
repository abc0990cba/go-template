package repository

// Ticket storage on the in-memory fake: types, tickets, approval steps,
// comments, and timeline events, mirroring the PostgreSQL adapter's
// semantics (store-assigned per-org numbers, immutable numbers on update,
// cascading deletes).

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/tickets"
)

// --- Tickets ------------------------------------------------------------

// CreateTicket stores the ticket and assigns its per-organization sequence
// number, mirroring the SQL insert's number subselect.
func (s *MemoryStore) CreateTicket(_ context.Context, ticket tickets.Ticket) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ticketTypes[ticket.TypeID]; !ok {
		return 0, kernel.ErrNotFound
	}
	if ticket.CreatedAt.IsZero() {
		ticket.CreatedAt = time.Now()
	}
	if ticket.UpdatedAt.IsZero() {
		ticket.UpdatedAt = ticket.CreatedAt
	}
	s.ticketSeq[ticket.OrgID]++
	ticket.Number = s.ticketSeq[ticket.OrgID]
	s.tickets[ticket.ID] = ticket
	return ticket.Number, nil
}

// GetTicket returns the ticket by id.
func (s *MemoryStore) GetTicket(_ context.Context, id string) (tickets.Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ticket, ok := s.tickets[id]
	if !ok {
		return tickets.Ticket{}, kernel.ErrNotFound
	}
	return ticket, nil
}

// UpdateTicket overwrites the ticket record. The per-organization number is
// store-assigned and immutable, matching the SQL UPDATE that never touches
// the column.
func (s *MemoryStore) UpdateTicket(_ context.Context, ticket tickets.Ticket) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.tickets[ticket.ID]
	if !ok {
		return kernel.ErrNotFound
	}
	// The copy is by-value, so keeping the stored number is invisible to
	// the caller's struct.
	ticket.Number = existing.Number
	s.tickets[ticket.ID] = ticket
	return nil
}

// DeleteTicket removes the ticket and cascades its approvals, comments, and
// events, like the foreign keys do on PostgreSQL.
func (s *MemoryStore) DeleteTicket(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tickets[id]; !ok {
		return kernel.ErrNotFound
	}
	delete(s.tickets, id)
	delete(s.approvals, id)
	comments := s.comments[:0]
	for _, comment := range s.comments {
		if comment.TicketID != id {
			comments = append(comments, comment)
		}
	}
	s.comments = comments
	events := s.ticketEvents[:0]
	for _, event := range s.ticketEvents {
		if event.TicketID != id {
			events = append(events, event)
		}
	}
	s.ticketEvents = events
	return nil
}

// ticketInScope applies the ticket listing filters. Caller holds the lock.
func ticketInScope(ticket tickets.Ticket, opts kernel.TicketOptions) bool {
	query := strings.ToLower(opts.Query)
	if opts.OrgID != "" && ticket.OrgID != opts.OrgID {
		return false
	}
	if opts.CreatedBy != "" && ticket.CreatedBy != opts.CreatedBy {
		return false
	}
	if opts.Status != "" && ticket.Status != opts.Status {
		return false
	}
	if opts.Type != "" && ticket.TypeKey != opts.Type {
		return false
	}
	if opts.Priority != "" && ticket.Priority != opts.Priority {
		return false
	}
	if query != "" && !strings.Contains(strings.ToLower(ticket.Title+" "+ticket.Description), query) {
		return false
	}
	return true
}

// ListTickets returns one page of tickets newest first plus the next-page
// cursor.
func (s *MemoryStore) ListTickets(_ context.Context, opts kernel.TicketOptions) ([]tickets.Ticket, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := make([]tickets.Ticket, 0, len(s.tickets))
	for _, ticket := range s.tickets {
		if ticketInScope(ticket, opts) {
			rows = append(rows, ticket)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	page, next := pageSlice(rows, func(ticket tickets.Ticket) string { return ticket.ID }, opts.ListOptions)
	return page, next, nil
}

// CountTicketsByStatus counts the scoped tickets per status.
func (s *MemoryStore) CountTicketsByStatus(_ context.Context, orgID, createdBy string) (map[string]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := map[string]int64{}
	for _, ticket := range s.tickets {
		if orgID != "" && ticket.OrgID != orgID {
			continue
		}
		if createdBy != "" && ticket.CreatedBy != createdBy {
			continue
		}
		counts[ticket.Status]++
	}
	return counts, nil
}

// --- Approval steps -----------------------------------------------------

// ReplaceApprovals rewrites the ticket's approval steps; an empty slice
// clears them (reopen).
func (s *MemoryStore) ReplaceApprovals(_ context.Context, ticketID string, steps []tickets.TicketApproval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tickets[ticketID]; !ok {
		return kernel.ErrNotFound
	}
	if steps == nil {
		steps = []tickets.TicketApproval{}
	}
	s.approvals[ticketID] = steps
	return nil
}

// ListApprovals returns the ticket's steps in order.
func (s *MemoryStore) ListApprovals(_ context.Context, ticketID string) ([]tickets.TicketApproval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	steps := s.approvals[ticketID]
	out := make([]tickets.TicketApproval, len(steps))
	copy(out, steps)
	return out, nil
}

// UpdateApproval rewrites one step's decision data.
func (s *MemoryStore) UpdateApproval(_ context.Context, approval tickets.TicketApproval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, step := range s.approvals[approval.TicketID] {
		if step.ID == approval.ID {
			s.approvals[approval.TicketID][i] = approval
			return nil
		}
	}
	return kernel.ErrNotFound
}

// ListTicketsForApprover pages in-review tickets whose current pending step
// matches one of the given roles.
func (s *MemoryStore) ListTicketsForApprover(_ context.Context, orgID string, roles []string, opts kernel.ListOptions) ([]tickets.Ticket, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	roleSet := make(map[string]bool, len(roles))
	for _, role := range roles {
		roleSet[role] = true
	}
	query := strings.ToLower(opts.Query)
	rows := make([]tickets.Ticket, 0, len(s.tickets))
	for _, ticket := range s.tickets {
		if ticket.Status != tickets.TicketInReview {
			continue
		}
		if orgID != "" && ticket.OrgID != orgID {
			continue
		}
		steps := s.approvals[ticket.ID]
		if ticket.CurrentStep >= len(steps) {
			continue
		}
		current := steps[ticket.CurrentStep]
		if current.Status != tickets.ApprovalPending || !roleSet[current.ApproverRole] {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(ticket.Title+" "+ticket.Description), query) {
			continue
		}
		rows = append(rows, ticket)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	page, next := pageSlice(rows, func(ticket tickets.Ticket) string { return ticket.ID }, opts)
	return page, next, nil
}

// CountTicketsForApprover counts the tickets waiting on the given roles.
func (s *MemoryStore) CountTicketsForApprover(_ context.Context, orgID string, roles []string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	roleSet := make(map[string]bool, len(roles))
	for _, role := range roles {
		roleSet[role] = true
	}
	var n int64
	for _, ticket := range s.tickets {
		if ticket.Status != tickets.TicketInReview {
			continue
		}
		if orgID != "" && ticket.OrgID != orgID {
			continue
		}
		steps := s.approvals[ticket.ID]
		if ticket.CurrentStep < len(steps) && steps[ticket.CurrentStep].Status == tickets.ApprovalPending && roleSet[steps[ticket.CurrentStep].ApproverRole] {
			n++
		}
	}
	return n, nil
}

// ListDecidedApprovals returns the user's past decisions paired with their
// tickets, newest decision first.
func (s *MemoryStore) ListDecidedApprovals(_ context.Context, userID string, limit, offset int) ([]tickets.TicketDecision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rows []tickets.TicketDecision
	for ticketID, steps := range s.approvals {
		ticket, ok := s.tickets[ticketID]
		if !ok {
			continue
		}
		for _, step := range steps {
			if step.DecidedBy == userID && !step.DecidedAt.IsZero() {
				rows = append(rows, tickets.TicketDecision{Approval: step, Ticket: ticket})
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i].Approval.DecidedAt, rows[j].Approval.DecidedAt
		if a.Equal(*b) {
			return rows[i].Approval.ID < rows[j].Approval.ID
		}
		return a.After(*b)
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
