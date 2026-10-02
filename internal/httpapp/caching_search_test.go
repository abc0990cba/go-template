package httpapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/observ"
	platformcache "github.com/your-org/go-template/internal/platform/cache"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/repository"
)

// newCachedServer builds the standard memory-store router with an in-process
// cache attached to the user and auth services.
func newCachedServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	logger := discardLogger()
	store := repository.NewMemoryStore()
	services := newTestContainer(t, store, nil, logger)
	mem := platformcache.NewMemory()
	services.Users.WithCache(mem).WithLogger(logger)
	services.Auth.WithCache(mem)
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}, realtime.NewHub()))
	t.Cleanup(server.Close)
	return server
}

func TestUserCacheInvalidatesOnUpdate(t *testing.T) {
	server := newCachedServer(t)
	admin := registerAndLogin(t, server, "cache-admin@example.com")
	target := registerAndLogin(t, server, "cache-target@example.com")
	targetID := userIDOf(t, server, target)

	// Prime the cache with the original record.
	status, _ := callJSON(t, server, http.MethodGet, "/api/users/"+targetID, "", admin)
	if status != http.StatusOK {
		t.Fatalf("get: got %d", status)
	}

	// Update through the admin path; invalidation must defeat the 30s TTL.
	status, _ = callJSON(t, server, http.MethodPut, "/api/users/"+targetID,
		`{"email":"cache-target@example.com","name":"Renamed","external_ref":""}`, admin)
	if status != http.StatusOK {
		t.Fatalf("update: got %d", status)
	}
	status, body := callJSON(t, server, http.MethodGet, "/api/users/"+targetID, "", admin)
	if status != http.StatusOK {
		t.Fatalf("get after update: got %d", status)
	}
	if name, _ := body["name"].(string); name != "Renamed" {
		t.Fatalf("cache served stale user after invalidation: %v", body)
	}
}

func TestETagShortCircuitsUnchangedLists(t *testing.T) {
	server := newCachedServer(t)
	admin := registerAndLogin(t, server, "etag-admin@example.com")

	etag := lastETag(t, server, admin)
	if etag == "" {
		t.Fatal("list response must carry an ETag")
	}

	// Conditional repeat: nothing changed → 304.
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/users?limit=5", nil)
	req.Header.Set("Authorization", "Bearer "+admin)
	req.Header.Set("If-None-Match", etag)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("conditional get: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional repeat: want 304, got %d", resp.StatusCode)
	}

	// A mutation changes the payload → fresh 200 with a new tag.
	registerAndLogin(t, server, "etag-extra@example.com")
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/api/users?limit=5", nil)
	req.Header.Set("Authorization", "Bearer "+admin)
	req.Header.Set("If-None-Match", etag)
	resp, err = server.Client().Do(req)
	if err != nil {
		t.Fatalf("after mutation: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("changed list: want 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("ETag"); got == "" || got == etag {
		t.Fatalf("changed list must carry a fresh ETag, got %q (was %q)", got, etag)
	}
}

// lastETag issues a plain GET and returns the ETag header.
func lastETag(t *testing.T, server *httptest.Server, token string) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/users?limit=5", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.Header.Get("ETag")
}

func TestSearchFindsUsersByPermission(t *testing.T) {
	server := newCachedServer(t)
	admin := registerAndLogin(t, server, "search-admin@example.com") // superadmin
	viewer := registerAndLogin(t, server, "search-viewer@example.com")

	status, _ := callJSON(t, server, http.MethodPost, "/api/users",
		`{"email":"zaphod@example.com","name":"Zaphod Beeble","external_ref":""}`, admin)
	if status != http.StatusCreated {
		t.Fatalf("create user: got %d", status)
	}

	// Superadmin (users:read) sees the users section.
	status, body := callJSON(t, server, http.MethodGet, "/api/search?q=beeble", "", admin)
	if status != http.StatusOK {
		t.Fatalf("search: got %d (%v)", status, body)
	}
	users, _ := body["users"].([]any)
	if len(users) != 1 {
		t.Fatalf("want one matching user, got %v", body)
	}
	if _, hasTickets := body["tickets"]; !hasTickets {
		t.Fatal("response must always carry both sections")
	}

	// The viewer role has tickets:read but not users:read: the users
	// section stays empty for that caller even though matches exist.
	status, body = callJSON(t, server, http.MethodGet, "/api/search?q=zaphod", "", viewer)
	if status != http.StatusOK {
		t.Fatalf("viewer search: got %d", status)
	}
	if users, _ := body["users"].([]any); len(users) != 0 {
		t.Fatalf("viewer must not search users, got %v", body)
	}
	if tickets, ok := body["tickets"].([]any); !ok {
		t.Fatal("tickets section must always serialize")
	} else if tickets == nil {
		t.Fatal("tickets section must serialize as [] not null")
	}
}
