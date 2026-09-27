package postgres

// Tickets domain on PostgreSQL: types with jsonb approval chains, the
// tickets themselves, and their steps, comments, and timeline events.

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/platform/postgres/db"
	"github.com/your-org/go-template/internal/repository"
)

// stepsToJSON renders the approval chain for the jsonb column.
func stepsToJSON(steps []model.TicketStep) string {
	raw, err := json.Marshal(steps)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// stepsFromJSON parses the stored chain; a corrupt value degrades to an
// empty chain instead of failing every read.
func stepsFromJSON(raw string) []model.TicketStep {
	var steps []model.TicketStep
	if err := json.Unmarshal([]byte(raw), &steps); err != nil || steps == nil {
		return []model.TicketStep{}
	}
	return steps
}

func ticketTypeFromRow(row db.TicketType) model.TicketType {
	return model.TicketType{
		ID: row.ID, OrgID: row.OrgID, Key: row.Key, Name: row.Name,
		Description: row.Description, Steps: stepsFromJSON(row.Steps), Active: row.Active,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

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

func ticketFromRow(row db.Ticket) model.Ticket {
	return model.Ticket{
		ID: row.ID, Number: int(row.Number), OrgID: row.OrgID, TypeID: row.TypeID,
		TypeKey: row.TypeKey, Title: row.Title, Description: row.Description,
		Priority: row.Priority, Status: row.Status, CurrentStep: int(row.CurrentStep),
		CreatedBy: row.CreatedBy, DecidedBy: row.DecidedBy, DecidedAt: tsPtr(row.DecidedAt),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// CreateTicketType stores one type; the (org_id, key) unique index guards
// duplicates.
func (s *Store) CreateTicketType(ctx context.Context, typ model.TicketType) error {
	return mapErr(s.q.InsertTicketType(ctx, db.InsertTicketTypeParams{
		ID: typ.ID, OrgID: typ.OrgID, Key: typ.Key, Name: typ.Name,
		Description: typ.Description, Column6: stepsToJSON(typ.Steps), Active: typ.Active,
		CreatedBy: typ.CreatedBy, CreatedAt: typ.CreatedAt, UpdatedAt: typ.UpdatedAt,
	}))
}

// GetTicketType returns the type by id.
func (s *Store) GetTicketType(ctx context.Context, id string) (model.TicketType, error) {
	row, err := s.q.GetTicketType(ctx, id)
	if err != nil {
		return model.TicketType{}, mapErr(err)
	}
	return ticketTypeFromRow(row), nil
}

// GetTicketTypeByKey resolves a type in one organization's namespace.
func (s *Store) GetTicketTypeByKey(ctx context.Context, orgID, key string) (model.TicketType, error) {
	row, err := s.q.GetTicketTypeByKey(ctx, db.GetTicketTypeByKeyParams{OrgID: orgID, Key: key})
	if err != nil {
		return model.TicketType{}, mapErr(err)
	}
	return ticketTypeFromRow(row), nil
}

// UpdateTicketType rewrites the mutable fields.
func (s *Store) UpdateTicketType(ctx context.Context, typ model.TicketType) error {
	return mapErr(s.q.UpdateTicketType(ctx, db.UpdateTicketTypeParams{
		ID: typ.ID, Name: typ.Name, Description: typ.Description,
		Column4: stepsToJSON(typ.Steps), Active: typ.Active,
	}))
}

// DeleteTicketType removes the type and reports ErrNotFound when absent.
func (s *Store) DeleteTicketType(ctx context.Context, id string) error {
	n, err := s.q.DeleteTicketType(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// ListTicketTypes returns the platform-wide types plus the organization's
// own, ordered by key.
func (s *Store) ListTicketTypes(ctx context.Context, orgID string) ([]model.TicketType, error) {
	rows, err := s.q.ListTicketTypes(ctx, orgID)
	if err != nil {
		return nil, mapErr(err)
	}
	types := make([]model.TicketType, 0, len(rows))
	for _, row := range rows {
		types = append(types, ticketTypeFromRow(row))
	}
	return types, nil
}

// CreateTicket stores the ticket; the INSERT assigns the per-organization
// sequence number and returns it.
func (s *Store) CreateTicket(ctx context.Context, ticket model.Ticket) (int, error) {
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
func (s *Store) GetTicket(ctx context.Context, id string) (model.Ticket, error) {
	row, err := s.q.GetTicket(ctx, id)
	if err != nil {
		return model.Ticket{}, mapErr(err)
	}
	return ticketFromRow(row), nil
}

// UpdateTicket rewrites the mutable columns.
func (s *Store) UpdateTicket(ctx context.Context, ticket model.Ticket) error {
	return mapErr(s.q.UpdateTicket(ctx, db.UpdateTicketParams{
		ID: ticket.ID, Title: ticket.Title, Description: ticket.Description,
		Priority: ticket.Priority, Status: ticket.Status,
		CurrentStep: int32(ticket.CurrentStep), // #nosec G115 -- step index is a small counter
		DecidedBy:   ticket.DecidedBy, DecidedAt: ts(ticket.DecidedAt),
	}))
}

// DeleteTicket removes the ticket (steps, comments, and events cascade).
func (s *Store) DeleteTicket(ctx context.Context, id string) error {
	n, err := s.q.DeleteTicket(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// ListTickets pages tickets newest first; cursor/keyset continue after
// After, and the returned cursor is empty on the last page.
func (s *Store) ListTickets(ctx context.Context, opts repository.TicketOptions) ([]model.Ticket, string, error) {
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
func ticketsPage(rows []db.Ticket, limit int32) ([]model.Ticket, string) {
	next := ""
	if len(rows) > int(limit) {
		rows = rows[:limit]
		next = rows[limit-1].ID
	}
	tickets := make([]model.Ticket, 0, len(rows))
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
func (s *Store) ReplaceApprovals(ctx context.Context, ticketID string, steps []model.TicketApproval) error {
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
func approvalFromRow(row db.TicketApproval) model.TicketApproval {
	return model.TicketApproval{
		ID: row.ID, TicketID: row.TicketID, StepIndex: int(row.StepIndex),
		Name: row.Name, ApproverRole: row.ApproverRole, Status: row.Status,
		DecidedBy: row.DecidedBy, DecidedAt: tsPtr(row.DecidedAt), Comment: row.Comment,
		CreatedAt: row.CreatedAt,
	}
}

// ListApprovals returns the ticket's steps in order.
func (s *Store) ListApprovals(ctx context.Context, ticketID string) ([]model.TicketApproval, error) {
	rows, err := s.q.ListTicketApprovals(ctx, ticketID)
	if err != nil {
		return nil, mapErr(err)
	}
	steps := make([]model.TicketApproval, 0, len(rows))
	for _, row := range rows {
		steps = append(steps, approvalFromRow(row))
	}
	return steps, nil
}

// UpdateApproval rewrites one step's decision data.
func (s *Store) UpdateApproval(ctx context.Context, approval model.TicketApproval) error {
	return mapErr(s.q.UpdateTicketApproval(ctx, db.UpdateTicketApprovalParams{
		ID: approval.ID, Status: approval.Status, DecidedBy: approval.DecidedBy,
		DecidedAt: ts(approval.DecidedAt), Comment: approval.Comment,
	}))
}

// ListTicketsForApprover pages in-review tickets whose current pending step
// matches one of the given roles.
func (s *Store) ListTicketsForApprover(ctx context.Context, orgID string, roles []string, opts repository.ListOptions) ([]model.Ticket, string, error) {
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
func (s *Store) ListDecidedApprovals(ctx context.Context, userID string, limit, offset int) ([]model.TicketDecision, error) {
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
	decisions := make([]model.TicketDecision, 0, len(rows))
	for _, row := range rows {
		decisions = append(decisions, model.TicketDecision{
			Approval: model.TicketApproval{
				ID: row.ID, TicketID: row.TicketID, StepIndex: int(row.StepIndex),
				Name: row.Name, ApproverRole: row.ApproverRole, Status: row.Status,
				DecidedBy: row.DecidedBy, DecidedAt: tsPtr(row.DecidedAt), Comment: row.Comment,
				CreatedAt: row.CreatedAt,
			},
			Ticket: model.Ticket{
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

// AddTicketComment appends one comment.
func (s *Store) AddTicketComment(ctx context.Context, comment model.TicketComment) error {
	return mapErr(s.q.InsertTicketComment(ctx, db.InsertTicketCommentParams{
		ID: comment.ID, TicketID: comment.TicketID, AuthorID: comment.AuthorID,
		Body: comment.Body, CreatedAt: comment.CreatedAt,
	}))
}

// ListTicketComments pages the ticket's comments newest first.
func (s *Store) ListTicketComments(ctx context.Context, ticketID string, limit, offset int) ([]model.TicketComment, error) {
	lim, off := pageBounds(limit, offset)
	rows, err := s.q.ListTicketComments(ctx, db.ListTicketCommentsParams{TicketID: ticketID, Lim: lim, Off: off})
	if err != nil {
		return nil, mapErr(err)
	}
	comments := make([]model.TicketComment, 0, len(rows))
	for _, row := range rows {
		comments = append(comments, model.TicketComment{
			ID: row.ID, TicketID: row.TicketID, AuthorID: row.AuthorID, Body: row.Body, CreatedAt: row.CreatedAt,
		})
	}
	return comments, nil
}

// AddTicketEvent appends one timeline entry.
func (s *Store) AddTicketEvent(ctx context.Context, event model.TicketEvent) error {
	return mapErr(s.q.InsertTicketEvent(ctx, db.InsertTicketEventParams{
		ID: event.ID, TicketID: event.TicketID, ActorID: event.ActorID,
		Action: event.Action, Detail: event.Detail, CreatedAt: event.CreatedAt,
	}))
}

// ListTicketEvents pages the ticket's timeline newest first.
func (s *Store) ListTicketEvents(ctx context.Context, ticketID string, limit, offset int) ([]model.TicketEvent, error) {
	lim, off := pageBounds(limit, offset)
	rows, err := s.q.ListTicketEvents(ctx, db.ListTicketEventsParams{TicketID: ticketID, Lim: lim, Off: off})
	if err != nil {
		return nil, mapErr(err)
	}
	events := make([]model.TicketEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, model.TicketEvent{
			ID: row.ID, TicketID: row.TicketID, ActorID: row.ActorID,
			Action: row.Action, Detail: row.Detail, CreatedAt: row.CreatedAt,
		})
	}
	return events, nil
}

// pageBounds clamps a limit/offset pair into sqlc's int32 arguments.
func pageBounds(limit, offset int) (int32, int32) {
	lim := int32(25)
	if limit > 0 && limit <= 100 {
		lim = int32(limit)
	}
	off := int32(0)
	if offset > 0 {
		off = int32(min(offset, 1<<31-1)) // #nosec G115 -- clamped to int32 range
	}
	return lim, off
}
