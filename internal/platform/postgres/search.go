package postgres

import (
	"context"

	"github.com/your-org/go-template/internal/platform/postgres/db"
	"github.com/your-org/go-template/internal/search"
	"github.com/your-org/go-template/internal/tickets"
	"github.com/your-org/go-template/internal/users"
)

func (s *Store) SearchUsers(ctx context.Context, query string, limit int) ([]users.User, error) {
	rows, err := s.q.SearchUsersByVector(ctx, db.SearchUsersByVectorParams{
		Query: query,
		Limit: int32(limit),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]users.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, users.User{
			ID:        row.ID,
			Email:     row.Email,
			Name:      row.Name,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

func (s *Store) SearchTickets(ctx context.Context, domain, query string, limit int) ([]tickets.Ticket, error) {
	rows, err := s.q.SearchTicketsByVector(ctx, db.SearchTicketsByVectorParams{
		OrgID: domain,
		Query: query,
		Limit: int32(limit),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]tickets.Ticket, 0, len(rows))
	for _, row := range rows {
		out = append(out, tickets.Ticket{
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

var _ search.Store = (*Store)(nil)
