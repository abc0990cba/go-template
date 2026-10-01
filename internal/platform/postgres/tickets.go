package postgres

// Tickets domain on PostgreSQL: types with jsonb approval chains, the
// tickets themselves, and their steps, comments, and timeline events.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/postgres/db"
	"github.com/your-org/go-template/internal/tickets"
)

// ts converts an optional time to the pgx nullable type.
func ts(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// tsPtr converts back; an invalid value reads as nil.
func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	value := t.Time
	return &value
}

func ticketFromRow(row db.Ticket) tickets.Ticket {
	return tickets.Ticket{
		ID: row.ID, Number: int(row.Number), OrgID: row.OrgID, TypeID: row.TypeID,
		TypeKey: row.TypeKey, Title: row.Title, Description: row.Description,
		Priority: row.Priority, Status: row.Status, CurrentStep: int(row.CurrentStep),
		CreatedBy: row.CreatedBy, DecidedBy: row.DecidedBy, DecidedAt: tsPtr(row.DecidedAt),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// CreateTicket stores the ticket; the INSERT assigns the per-organization
// sequence number and returns it.
func (s *Store) CreateTicket(ctx context.Context, ticket tickets.Ticket) (int, error) {
	number, err := s.q.InsertTicket(ctx, db.InsertTicketParams{
		ID: ticket.ID, OrgID: ticket.OrgID, TypeID: ticket.TypeID, TypeKey: ticket.TypeKey,
		Title: ticket.Title, Description: ticket.Description, Priority: ticket.Priority,
		Status: ticket.Status, CurrentStep: int32(ticket.CurrentStep), // #nosec G115 -- step index is a small counter
		CreatedBy: ticket.CreatedBy, CreatedAt: ticket.CreatedAt, UpdatedAt: ticket.UpdatedAt,
	})
	if err != nil {
		return 0, mapErr(err)
	}
	return int(number), nil
}

// GetTicket returns the ticket by id.
func (s *Store) GetTicket(ctx context.Context, id string) (tickets.Ticket, error) {
	row, err := s.q.GetTicket(ctx, id)
	if err != nil {
		return tickets.Ticket{}, mapErr(err)
	}
	return ticketFromRow(row), nil
}

// UpdateTicket rewrites the mutable columns.
func (s *Store) UpdateTicket(ctx context.Context, ticket tickets.Ticket) error {
	n, err := s.q.UpdateTicket(ctx, db.UpdateTicketParams{
		ID: ticket.ID, Title: ticket.Title, Description: ticket.Description,
		Priority: ticket.Priority, Status: ticket.Status,
		CurrentStep: int32(ticket.CurrentStep), // #nosec G115 -- step index is a small counter
		DecidedBy:   ticket.DecidedBy, DecidedAt: ts(ticket.DecidedAt),
	})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return fmt.Errorf("%w: ticket not found", kernel.ErrNotFound)
	}
	return nil
}

// DeleteTicket removes the ticket (steps, comments, and events cascade).
func (s *Store) DeleteTicket(ctx context.Context, id string) error {
	n, err := s.q.DeleteTicket(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return kernel.ErrNotFound
	}
	return nil
}

// ListTickets pages tickets newest first; cursor/keyset continue after
// After, and the returned cursor is empty on the last page.
func (s *Store) ListTickets(ctx context.Context, opts kernel.TicketOptions) ([]tickets.Ticket, string, error) {
	const def = 25
	limit := int32(def)
	if opts.Limit > 0 && opts.Limit <= 100 {
		limit = int32(opts.Limit)
	}
	if opts.Mode == "cursor" || opts.Mode == "keyset" {
		if opts.After != "" {
			rows, err := s.q.ListTicketsKeyset(ctx, db.ListTicketsKeysetParams{
				Query: opts.Query, Org: opts.OrgID, Creator: opts.CreatedBy,
				Status: opts.Status, Typ: opts.Type, Priority: opts.Priority,
				After: opts.After, Lim: limit + 1,
			})
			if err != nil {
				return nil, "", mapErr(err)
			}
			page, next := ticketsPage(rows, limit)
			return page, next, nil
		}
	}
	off := int32(0)
	if opts.Offset > 0 {
		off = int32(min(opts.Offset, 1<<31-1)) // #nosec G115 -- clamped to int32 range
	}
	rows, err := s.q.ListTicketsOffset(ctx, db.ListTicketsOffsetParams{
		Query: opts.Query, Org: opts.OrgID, Creator: opts.CreatedBy,
		Status: opts.Status, Typ: opts.Type, Priority: opts.Priority, Off: off, Lim: limit + 1,
	})
	if err != nil {
		return nil, "", mapErr(err)
	}
	page, next := ticketsPage(rows, limit)
	return page, next, nil
}

// ticketsPage trims the limit+1th row and derives the next cursor.
func ticketsPage(rows []db.Ticket, limit int32) ([]tickets.Ticket, string) {
	next := ""
	if len(rows) > int(limit) {
		rows = rows[:limit]
		next = rows[limit-1].ID
	}
	tickets := make([]tickets.Ticket, 0, len(rows))
	for _, row := range rows {
		tickets = append(tickets, ticketFromRow(row))
	}
	return tickets, next
}

// CountTicketsByStatus counts the scoped tickets per status.
func (s *Store) CountTicketsByStatus(ctx context.Context, orgID, createdBy string) (map[string]int64, error) {
	rows, err := s.q.CountTicketsByStatus(ctx, db.CountTicketsByStatusParams{Org: orgID, Creator: createdBy})
	if err != nil {
		return nil, mapErr(err)
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.Status] = row.N
	}
	return counts, nil
}

// ReplaceApprovals rewrites the ticket's steps in one transaction: the old
// chain goes away, then every step is inserted fresh.
func (s *Store) ReplaceApprovals(ctx context.Context, ticketID string, steps []tickets.TicketApproval) error {
	return s.tx(ctx, func(q *db.Queries) error {
		if err := q.DeleteTicketApprovals(ctx, ticketID); err != nil {
			return mapErr(err)
		}
		for _, step := range steps {
			decidedAt := pgtype.Timestamptz{}
			if step.DecidedAt != nil {
				decidedAt = pgtype.Timestamptz{Time: *step.DecidedAt, Valid: true}
			}
			if err := q.InsertTicketApproval(ctx, db.InsertTicketApprovalParams{
				ID: step.ID, TicketID: step.TicketID, StepIndex: int32(step.StepIndex), // #nosec G115 -- step index is a small counter
				Name: step.Name, ApproverRole: step.ApproverRole, Status: step.Status,
				DecidedBy: step.DecidedBy, DecidedAt: decidedAt, Comment: step.Comment,
				CreatedAt: step.CreatedAt,
			}); err != nil {
				return mapErr(err)
			}
		}
		return nil
	})
}

// approvalFromRow converts one stored step.
func approvalFromRow(row db.TicketApproval) tickets.TicketApproval {
	return tickets.TicketApproval{
		ID: row.ID, TicketID: row.TicketID, StepIndex: int(row.StepIndex),
		Name: row.Name, ApproverRole: row.ApproverRole, Status: row.Status,
		DecidedBy: row.DecidedBy, DecidedAt: tsPtr(row.DecidedAt), Comment: row.Comment,
		CreatedAt: row.CreatedAt,
	}
}

// ListApprovals returns the ticket's steps in order.
func (s *Store) ListApprovals(ctx context.Context, ticketID string) ([]tickets.TicketApproval, error) {
	rows, err := s.q.ListTicketApprovals(ctx, ticketID)
	if err != nil {
		return nil, mapErr(err)
	}
	steps := make([]tickets.TicketApproval, 0, len(rows))
	for _, row := range rows {
		steps = append(steps, approvalFromRow(row))
	}
	return steps, nil
}

// UpdateApproval rewrites one step's decision data.
func (s *Store) UpdateApproval(ctx context.Context, approval tickets.TicketApproval) error {
	return mapErr(s.q.UpdateTicketApproval(ctx, db.UpdateTicketApprovalParams{
		ID: approval.ID, Status: approval.Status, DecidedBy: approval.DecidedBy,
		DecidedAt: ts(approval.DecidedAt), Comment: approval.Comment,
	}))
}

// ListTicketsForApprover pages in-review tickets whose current pending step
// matches one of the given roles.
func (s *Store) ListTicketsForApprover(ctx context.Context, orgID string, roles []string, opts kernel.ListOptions) ([]tickets.Ticket, string, error) {
	const def = 25
	limit := int32(def)
	if opts.Limit > 0 && opts.Limit <= 100 {
		limit = int32(opts.Limit)
	}
	if opts.Mode == "cursor" || opts.Mode == "keyset" {
		if opts.After != "" {
			rows, err := s.q.ListTicketsForApproverKeyset(ctx, db.ListTicketsForApproverKeysetParams{
				Org: orgID, Roles: roles, Query: opts.Query, After: opts.After, Lim: limit + 1,
			})
			if err != nil {
				return nil, "", mapErr(err)
			}
			page, next := ticketsPage(rows, limit)
			return page, next, nil
		}
	}
	off := int32(0)
	if opts.Offset > 0 {
		off = int32(min(opts.Offset, 1<<31-1)) // #nosec G115 -- clamped to int32 range
	}
	rows, err := s.q.ListTicketsForApproverOffset(ctx, db.ListTicketsForApproverOffsetParams{
		Org: orgID, Roles: roles, Query: opts.Query, Off: off, Lim: limit + 1,
	})
	if err != nil {
		return nil, "", mapErr(err)
	}
	page, next := ticketsPage(rows, limit)
	return page, next, nil
}

// CountTicketsForApprover counts the tickets waiting on the given roles.
func (s *Store) CountTicketsForApprover(ctx context.Context, orgID string, roles []string) (int64, error) {
	return s.q.CountTicketsForApprover(ctx, db.CountTicketsForApproverParams{Org: orgID, Roles: roles})
}

// ListDecidedApprovals returns the user's past decisions with their ticket
// snapshots, newest first.
func (s *Store) ListDecidedApprovals(ctx context.Context, userID string, limit, offset int) ([]tickets.TicketDecision, error) {
	lim := int32(25)
	if limit > 0 && limit <= 100 {
		lim = int32(limit)
	}
	off := int32(0)
	if offset > 0 {
		off = int32(min(offset, 1<<31-1)) // #nosec G115 -- clamped to int32 range
	}
	rows, err := s.q.ListDecidedApprovals(ctx, db.ListDecidedApprovalsParams{DecidedBy: userID, Off: off, Lim: lim})
	if err != nil {
		return nil, mapErr(err)
	}
	decisions := make([]tickets.TicketDecision, 0, len(rows))
	for _, row := range rows {
		decisions = append(decisions, tickets.TicketDecision{
			Approval: tickets.TicketApproval{
				ID: row.ID, TicketID: row.TicketID, StepIndex: int(row.StepIndex),
				Name: row.Name, ApproverRole: row.ApproverRole, Status: row.Status,
				DecidedBy: row.DecidedBy, DecidedAt: tsPtr(row.DecidedAt), Comment: row.Comment,
				CreatedAt: row.CreatedAt,
			},
			Ticket: tickets.Ticket{
				ID: row.TicketID, Number: int(row.Number), OrgID: row.OrgID, TypeID: row.TypeID,
				TypeKey: row.TypeKey, Title: row.Title, Description: row.Description,
				Priority: row.Priority, Status: row.Status_2, CurrentStep: int(row.CurrentStep),
				CreatedBy: row.CreatedBy, DecidedBy: row.DecidedBy_2,
				DecidedAt: tsPtr(row.TicketDecidedAt), CreatedAt: row.TicketCreatedAt, UpdatedAt: row.TicketUpdatedAt,
			},
		})
	}
	return decisions, nil
}
