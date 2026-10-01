package repository

import (
	"context"
	"strings"

	"github.com/your-org/go-template/internal/search"
	"github.com/your-org/go-template/internal/tickets"
	"github.com/your-org/go-template/internal/users"
)

// Full-text search: case-insensitive substring stands in for the tsvector
// query the PostgreSQL adapter runs, with the same call shape.

func (s *MemoryStore) SearchUsers(_ context.Context, query string, limit int) ([]users.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	needle := strings.ToLower(query)
	out := make([]users.User, 0, limit)
	for _, user := range s.users {
		if len(out) >= limit {
			break
		}
		if strings.Contains(strings.ToLower(user.Name), needle) || strings.Contains(strings.ToLower(user.Email), needle) {
			out = append(out, user)
		}
	}
	return out, nil
}

func (s *MemoryStore) SearchTickets(_ context.Context, domain, query string, limit int) ([]tickets.Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	needle := strings.ToLower(query)
	out := make([]tickets.Ticket, 0, limit)
	for _, ticket := range s.tickets {
		if len(out) >= limit {
			break
		}
		if domain != "" && domain != "*" && ticket.OrgID != domain {
			continue
		}
		if strings.Contains(strings.ToLower(ticket.Title), needle) || strings.Contains(strings.ToLower(ticket.Description), needle) {
			out = append(out, ticket)
		}
	}
	return out, nil
}

var _ search.Store = (*MemoryStore)(nil)
