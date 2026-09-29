package service

import (
	"context"
	"strings"

	"github.com/your-org/go-template/internal/model"
)

// Search limits: the endpoint is an autocomplete-grade feed, not a listing
// replacement.
const (
	SearchDefaultLimit = 10
	SearchMaxLimit     = 25
)

// SearchResults is the combined search response; empty slices serialize as
// [] so clients can iterate blindly.
type SearchResults struct {
	Users   []model.User   `json:"users"`
	Tickets []model.Ticket `json:"tickets"`
}

// SearchService runs permission-scoped full-text search: users results need
// users:read, tickets results need tickets:read, and without tickets:read_all
// only the caller's own tickets come back (matching the list scope rules).
type SearchService struct {
	store SearchRepo
	rbac  *RBACService
}

// NewSearchService wires the search facade.
func NewSearchService(store SearchRepo, rbac *RBACService) *SearchService {
	return &SearchService{store: store, rbac: rbac}
}

// Search runs the query within the active domain and returns only the
// sections the caller may see. An empty query yields empty results, not an
// error.
func (s *SearchService) Search(ctx context.Context, callerID, domain, query string, limit int) (SearchResults, error) {
	out := SearchResults{Users: []model.User{}, Tickets: []model.Ticket{}}
	query = strings.TrimSpace(query)
	if query == "" {
		return out, nil
	}
	if limit <= 0 {
		limit = SearchDefaultLimit
	}
	if limit > SearchMaxLimit {
		limit = SearchMaxLimit
	}
	if s.rbac.Enforce(callerID, domain, "users", "read") {
		users, err := s.store.SearchUsers(ctx, query, limit)
		if err != nil {
			return out, err
		}
		if users == nil {
			users = []model.User{}
		}
		out.Users = users
	}
	if s.rbac.Enforce(callerID, domain, "tickets", "read") {
		tickets, err := s.store.SearchTickets(ctx, domain, query, limit)
		if err != nil {
			return out, err
		}
		if !s.rbac.Enforce(callerID, domain, "tickets", "read_all") {
			own := tickets[:0]
			for _, ticket := range tickets {
				if ticket.CreatedBy == callerID {
					own = append(own, ticket)
				}
			}
			tickets = own
		}
		if tickets == nil {
			tickets = []model.Ticket{}
		}
		out.Tickets = tickets
	}
	return out, nil
}
