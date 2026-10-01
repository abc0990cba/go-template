package search_test

import (
	"context"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/kernel"
	platformrbac "github.com/your-org/go-template/internal/platform/rbac"
	"github.com/your-org/go-template/internal/rbac"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/search"
	"github.com/your-org/go-template/internal/tickets"
)

// newRBACService builds the rbac feature service over a memory-backed
// policy store with the system roles seeded, mirroring production wiring.
// The enforcer comes from the production adapter: the external test package
// may import platform/rbac (repository imports search, so a same-package
// test using MemoryStore would cycle).
func newRBACService(t *testing.T) (*rbac.Service, *repository.MemoryStore) {
	t.Helper()
	store := repository.NewMemoryStore()
	enforcer, err := platformrbac.NewEnforcer(store)
	if err != nil {
		t.Fatalf("enforcer: %v", err)
	}
	svc := rbac.NewService(enforcer, store, store, nil)
	if err := svc.EnsureSystemRoles(t.Context()); err != nil {
		t.Fatalf("seed system roles: %v", err)
	}
	return svc, store
}

func TestSearchScopesTicketsToOwnWithoutReadAll(t *testing.T) {
	svc, store := newRBACService(t)
	ctx := context.Background()
	authSvc := auth.NewService(store, nil, time.Hour)
	admin, _, _ := authSvc.Register(ctx, "search-admin2@example.com", "Admin", "password123")
	requester, _, _ := authSvc.Register(ctx, "search-requester2@example.com", "Requester", "password123")
	other, _, _ := authSvc.Register(ctx, "search-other2@example.com", "Other", "password123")
	if err := svc.AssignRole(admin.ID, "superadmin", rbac.PlatformDomain); err != nil {
		t.Fatalf("grant superadmin: %v", err)
	}
	if err := svc.AssignRole(requester.ID, "requester", rbac.PlatformDomain); err != nil {
		t.Fatalf("grant requester: %v", err)
	}

	// A ticket type is a foreign key for tickets.
	typ := tickets.TicketType{ID: kernel.NewID(), OrgID: "", Key: "audit", Name: "Audit", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := store.CreateTicketType(ctx, typ); err != nil {
		t.Fatalf("seed type: %v", err)
	}
	for _, seed := range []struct{ title, createdBy string }{
		{"Quarterly audit paperwork", requester.ID},
		{"Audit trail cleanup", other.ID},
	} {
		_, err := store.CreateTicket(ctx, tickets.Ticket{
			ID: kernel.NewID(), OrgID: "", TypeID: typ.ID, Title: seed.title, CreatedBy: seed.createdBy,
		})
		if err != nil {
			t.Fatalf("seed ticket: %v", err)
		}
	}

	searchSvc := search.NewService(store, svc)

	// Superadmin holds tickets:read_all → both tickets.
	results, err := searchSvc.Search(ctx, admin.ID, rbac.PlatformDomain, "audit", 10)
	if err != nil {
		t.Fatalf("admin search: %v", err)
	}
	if len(results.Tickets) != 2 {
		t.Fatalf("admin sees all tickets, got %d", len(results.Tickets))
	}
	// A user-matching query fills the users section (superadmin has users:read).
	results, err = searchSvc.Search(ctx, admin.ID, rbac.PlatformDomain, "search-admin", 10)
	if err != nil {
		t.Fatalf("admin user search: %v", err)
	}
	if len(results.Users) == 0 {
		t.Fatal("admin (users:read) must get the users section")
	}

	// The requester (tickets:read, no read_all) sees only their own.
	results, err = searchSvc.Search(ctx, requester.ID, rbac.PlatformDomain, "audit", 10)
	if err != nil {
		t.Fatalf("requester search: %v", err)
	}
	if len(results.Tickets) != 1 || results.Tickets[0].CreatedBy != requester.ID {
		t.Fatalf("requester must see only own tickets, got %+v", results.Tickets)
	}

	// A user without roles gets empty sections, never an error.
	stranger, _, _ := authSvc.Register(ctx, "search-stranger2@example.com", "Stranger", "password123")
	results, err = searchSvc.Search(ctx, stranger.ID, rbac.PlatformDomain, "audit", 10)
	if err != nil || len(results.Tickets) != 0 || len(results.Users) != 0 {
		t.Fatalf("stranger must get empty results: %v %+v", err, results)
	}

	// Empty query → empty sections, no error.
	results, err = searchSvc.Search(ctx, admin.ID, rbac.PlatformDomain, "   ", 10)
	if err != nil || len(results.Users) != 0 || len(results.Tickets) != 0 {
		t.Fatalf("empty query: %v %+v", err, results)
	}
}
