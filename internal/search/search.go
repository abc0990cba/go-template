// Package search runs the combined, permission-scoped full-text search
// across users and tickets: the service decides which sections the caller
// may see.
package search

import (
	"context"
	"strings"

	"github.com/your-org/go-template/internal/rbac"
	"github.com/your-org/go-template/internal/tickets"
	"github.com/your-org/go-template/internal/users"
)

// Search limits: the endpoint is an autocomplete-grade feed, not a listing
// replacement.
const (
	DefaultLimit = 10
	MaxLimit     = 25
)

// Store serves ranked full-text search over the primary entities. The
// Postgres adapter uses tsvector + websearch_to_tsquery; the memory fake
// does case-insensitive substring matching with the same call shape.
type Store interface {
	SearchUsers(ctx context.Context, query string, limit int) ([]users.User, error)
	SearchTickets(ctx context.Context, domain, query string, limit int) ([]tickets.Ticket, error)
}

// Results is the combined search response; empty slices serialize as
// [] so clients can iterate blindly.
type Results struct {
	Users   []users.User     `json:"users"`
	Tickets []tickets.Ticket `json:"tickets"`
}

// Service runs permission-scoped full-text search: users results need
// users:read, tickets results need tickets:read, and without tickets:read_all
// only the caller's own tickets come back (matching the list scope rules).
type Service struct {
	store Store
	rbac  *rbac.Service
}

// NewService wires the search facade.
func NewService(store Store, rbacSvc *rbac.Service) *Service {
	return &Service{store: store, rbac: rbacSvc}
}

// Search runs the query within the active domain and returns only the
// sections the caller may see. An empty query yields empty results, not an
// error.
func (s *Service) Search(ctx context.Context, callerID, domain, query string, limit int) (Results, error) {
	out := Results{Users: []users.User{}, Tickets: []tickets.Ticket{}}
	query = strings.TrimSpace(query)
	if query == "" {
		return out, nil
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	if s.rbac.Enforce(callerID, domain, "users", "read") {
		matches, err := s.store.SearchUsers(ctx, query, limit)
		if err != nil {
			return out, err
		}
		if matches == nil {
			matches = []users.User{}
		}
		out.Users = matches
	}
	if s.rbac.Enforce(callerID, domain, "tickets", "read") {
		hits, err := s.store.SearchTickets(ctx, domain, query, limit)
		if err != nil {
			return out, err
		}
		if !s.rbac.Enforce(callerID, domain, "tickets", "read_all") {
			own := hits[:0]
			for _, ticket := range hits {
				if ticket.CreatedBy == callerID {
					own = append(own, ticket)
				}
			}
			hits = own
		}
		if hits == nil {
			hits = []tickets.Ticket{}
		}
		out.Tickets = hits
	}
	return out, nil
}
