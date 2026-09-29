package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/platform/cache"
)

func TestSearchScopesTicketsToOwnWithoutReadAll(t *testing.T) {
	svc, store := newRBACService(t)
	ctx := context.Background()
	auth := &AuthService{store: store, sessionTTL: time.Hour}
	admin, _, _ := auth.Register(ctx, "search-admin2@example.com", "Admin", "password123")
	requester, _, _ := auth.Register(ctx, "search-requester2@example.com", "Requester", "password123")
	other, _, _ := auth.Register(ctx, "search-other2@example.com", "Other", "password123")
	if err := svc.AssignRole(admin.ID, "superadmin", PlatformDomain); err != nil {
		t.Fatalf("grant superadmin: %v", err)
	}
	if err := svc.AssignRole(requester.ID, "requester", PlatformDomain); err != nil {
		t.Fatalf("grant requester: %v", err)
	}

	// A ticket type is a foreign key for tickets.
	typ := model.TicketType{ID: newID(), OrgID: "", Key: "audit", Name: "Audit", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := store.CreateTicketType(ctx, typ); err != nil {
		t.Fatalf("seed type: %v", err)
	}
	for _, seed := range []struct{ title, createdBy string }{
		{"Quarterly audit paperwork", requester.ID},
		{"Audit trail cleanup", other.ID},
	} {
		_, err := store.CreateTicket(ctx, model.Ticket{
			ID: newID(), OrgID: "", TypeID: typ.ID, Title: seed.title, CreatedBy: seed.createdBy,
		})
		if err != nil {
			t.Fatalf("seed ticket: %v", err)
		}
	}

	search := NewSearchService(store, svc)

	// Superadmin holds tickets:read_all → both tickets.
	results, err := search.Search(ctx, admin.ID, PlatformDomain, "audit", 10)
	if err != nil {
		t.Fatalf("admin search: %v", err)
	}
	if len(results.Tickets) != 2 {
		t.Fatalf("admin sees all tickets, got %d", len(results.Tickets))
	}
	// A user-matching query fills the users section (superadmin has users:read).
	results, err = search.Search(ctx, admin.ID, PlatformDomain, "search-admin", 10)
	if err != nil {
		t.Fatalf("admin user search: %v", err)
	}
	if len(results.Users) == 0 {
		t.Fatal("admin (users:read) must get the users section")
	}

	// The requester (tickets:read, no read_all) sees only their own.
	results, err = search.Search(ctx, requester.ID, PlatformDomain, "audit", 10)
	if err != nil {
		t.Fatalf("requester search: %v", err)
	}
	if len(results.Tickets) != 1 || results.Tickets[0].CreatedBy != requester.ID {
		t.Fatalf("requester must see only own tickets, got %+v", results.Tickets)
	}

	// A user without roles gets empty sections, never an error.
	stranger, _, _ := auth.Register(ctx, "search-stranger2@example.com", "Stranger", "password123")
	results, err = search.Search(ctx, stranger.ID, PlatformDomain, "audit", 10)
	if err != nil || len(results.Tickets) != 0 || len(results.Users) != 0 {
		t.Fatalf("stranger must get empty results: %v %+v", err, results)
	}

	// Empty query → empty sections, no error.
	results, err = search.Search(ctx, admin.ID, PlatformDomain, "   ", 10)
	if err != nil || len(results.Users) != 0 || len(results.Tickets) != 0 {
		t.Fatalf("empty query: %v %+v", err, results)
	}
}

func TestCachedGetOrLoadFillsAndHits(t *testing.T) {
	mem := cache.NewMemory()
	group := &singleflight.Group{}
	ctx := context.Background()
	calls := 0
	load := func(context.Context) (string, error) {
		calls++
		return "value", nil
	}

	for range 3 {
		value, err := CachedGetOrLoad(ctx, mem, nil, group, "k", time.Minute, load)
		if err != nil || value != "value" {
			t.Fatalf("load: %v %v", value, err)
		}
	}
	if calls != 1 {
		t.Fatalf("loader must run once, ran %d times", calls)
	}

	// Loader errors are not cached.
	failing := func(context.Context) (string, error) { return "", errors.New("boom") }
	if _, err := CachedGetOrLoad(ctx, mem, nil, group, "k2", time.Minute, failing); err == nil {
		t.Fatal("want loader error")
	}
	if _, ok := mem.Get(ctx, "k2"); ok {
		t.Fatal("errors must not be cached")
	}
}

func TestCachedGetOrLoadSingleflight(t *testing.T) {
	mem := cache.NewMemory()
	group := &singleflight.Group{}
	ctx := context.Background()
	var mu sync.Mutex
	calls := 0
	start := make(chan struct{})
	release := make(chan struct{})
	load := func(context.Context) (string, error) {
		mu.Lock()
		calls++
		first := calls == 1
		mu.Unlock()
		if first {
			close(start)
			<-release
		}
		return "shared", nil
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = CachedGetOrLoad(ctx, mem, nil, group, "hot", time.Minute, load)
		}()
	}
	<-start
	close(release)
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if calls > 2 {
		t.Fatalf("singleflight must collapse concurrent misses, loader ran %d times", calls)
	}
}
