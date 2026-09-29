package postgres

import (
	"context"
	"time"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/platform/postgres/db"
)

func (s *Store) SearchUsers(ctx context.Context, query string, limit int) ([]model.User, error) {
	rows, err := s.q.SearchUsersByVector(ctx, db.SearchUsersByVectorParams{
		Query: query,
		Limit: int32(limit),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]model.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.User{
			ID:        row.ID,
			Email:     row.Email,
			Name:      row.Name,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

func (s *Store) SearchTickets(ctx context.Context, domain, query string, limit int) ([]model.Ticket, error) {
	rows, err := s.q.SearchTicketsByVector(ctx, db.SearchTicketsByVectorParams{
		OrgID: domain,
		Query: query,
		Limit: int32(limit),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]model.Ticket, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.Ticket{
			ID:        row.ID,
			Number:    int(row.Number),
			OrgID:     row.OrgID,
			TypeKey:   row.TypeKey,
			Title:     row.Title,
			Status:    row.Status,
			Priority:  row.Priority,
			CreatedBy: row.CreatedBy,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

var _ = time.Time{}
