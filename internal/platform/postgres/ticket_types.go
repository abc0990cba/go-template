package postgres

import (
	"context"
	"encoding/json"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/postgres/db"
	"github.com/your-org/go-template/internal/tickets"
)

// stepsToJSON renders the approval chain for the jsonb column.
func stepsToJSON(steps []tickets.TicketStep) string {
	raw, err := json.Marshal(steps)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// stepsFromJSON parses the stored chain; a corrupt value degrades to an
// empty chain instead of failing every read.
func stepsFromJSON(raw string) []tickets.TicketStep {
	var steps []tickets.TicketStep
	if err := json.Unmarshal([]byte(raw), &steps); err != nil || steps == nil {
		return []tickets.TicketStep{}
	}
	return steps
}

func ticketTypeFromRow(row db.TicketType) tickets.TicketType {
	return tickets.TicketType{
		ID: row.ID, OrgID: row.OrgID, Key: row.Key, Name: row.Name,
		Description: row.Description, Steps: stepsFromJSON(row.Steps), Active: row.Active,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// CreateTicketType stores one type; the (org_id, key) unique index guards
// duplicates.
func (s *Store) CreateTicketType(ctx context.Context, typ tickets.TicketType) error {
	return mapErr(s.q.InsertTicketType(ctx, db.InsertTicketTypeParams{
		ID: typ.ID, OrgID: typ.OrgID, Key: typ.Key, Name: typ.Name,
		Description: typ.Description, Column6: stepsToJSON(typ.Steps), Active: typ.Active,
		CreatedBy: typ.CreatedBy, CreatedAt: typ.CreatedAt, UpdatedAt: typ.UpdatedAt,
	}))
}

// GetTicketType returns the type by id.
func (s *Store) GetTicketType(ctx context.Context, id string) (tickets.TicketType, error) {
	row, err := s.q.GetTicketType(ctx, id)
	if err != nil {
		return tickets.TicketType{}, mapErr(err)
	}
	return ticketTypeFromRow(row), nil
}

// GetTicketTypeByKey resolves a type in one organization's namespace.
func (s *Store) GetTicketTypeByKey(ctx context.Context, orgID, key string) (tickets.TicketType, error) {
	row, err := s.q.GetTicketTypeByKey(ctx, db.GetTicketTypeByKeyParams{OrgID: orgID, Key: key})
	if err != nil {
		return tickets.TicketType{}, mapErr(err)
	}
	return ticketTypeFromRow(row), nil
}

// UpdateTicketType rewrites the mutable fields.
func (s *Store) UpdateTicketType(ctx context.Context, typ tickets.TicketType) error {
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
		return kernel.ErrNotFound
	}
	return nil
}

// ListTicketTypes returns the platform-wide types plus the organization's
// own, ordered by key.
func (s *Store) ListTicketTypes(ctx context.Context, orgID string) ([]tickets.TicketType, error) {
	rows, err := s.q.ListTicketTypes(ctx, orgID)
	if err != nil {
		return nil, mapErr(err)
	}
	types := make([]tickets.TicketType, 0, len(rows))
	for _, row := range rows {
		types = append(types, ticketTypeFromRow(row))
	}
	return types, nil
}

// AddTicketComment appends one comment.
func (s *Store) AddTicketComment(ctx context.Context, comment tickets.TicketComment) error {
	return mapErr(s.q.InsertTicketComment(ctx, db.InsertTicketCommentParams{
		ID: comment.ID, TicketID: comment.TicketID, AuthorID: comment.AuthorID,
		Body: comment.Body, CreatedAt: comment.CreatedAt,
	}))
}

// ListTicketComments pages the ticket's comments newest first.
func (s *Store) ListTicketComments(ctx context.Context, ticketID string, limit, offset int) ([]tickets.TicketComment, error) {
	lim, off := pageBounds(limit, offset)
	rows, err := s.q.ListTicketComments(ctx, db.ListTicketCommentsParams{TicketID: ticketID, Lim: lim, Off: off})
	if err != nil {
		return nil, mapErr(err)
	}
	comments := make([]tickets.TicketComment, 0, len(rows))
	for _, row := range rows {
		comments = append(comments, tickets.TicketComment{
			ID: row.ID, TicketID: row.TicketID, AuthorID: row.AuthorID, Body: row.Body, CreatedAt: row.CreatedAt,
		})
	}
	return comments, nil
}

// AddTicketEvent appends one timeline entry.
func (s *Store) AddTicketEvent(ctx context.Context, event tickets.TicketEvent) error {
	return mapErr(s.q.InsertTicketEvent(ctx, db.InsertTicketEventParams{
		ID: event.ID, TicketID: event.TicketID, ActorID: event.ActorID,
		Action: event.Action, Detail: event.Detail, CreatedAt: event.CreatedAt,
	}))
}

// ListTicketEvents pages the ticket's timeline newest first.
func (s *Store) ListTicketEvents(ctx context.Context, ticketID string, limit, offset int) ([]tickets.TicketEvent, error) {
	lim, off := pageBounds(limit, offset)
	rows, err := s.q.ListTicketEvents(ctx, db.ListTicketEventsParams{TicketID: ticketID, Lim: lim, Off: off})
	if err != nil {
		return nil, mapErr(err)
	}
	events := make([]tickets.TicketEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, tickets.TicketEvent{
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
