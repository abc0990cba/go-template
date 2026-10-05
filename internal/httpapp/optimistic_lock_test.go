package httpapp

// Router-level tests for optimistic locking: the ETag a GET returns must be
// accepted by the matching write, and a stale tag must lose with 412 before
// the store is touched.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// getETag fetches path and returns the strong ETag header.
func getETag(t *testing.T, server *httptest.Server, path, token string) string {
	t.Helper()
	_, header, _ := rawRequest(t, server, http.MethodGet, path, "", token, nil)
	return header.Get("ETag")
}

// requestHeaders sends a body with extra headers and returns status + raw body.
func requestHeaders(t *testing.T, server *httptest.Server, method, path, body, token string, headers map[string]string) (int, string) {
	t.Helper()
	status, _, raw := rawRequest(t, server, method, path, body, token, headers)
	return status, raw
}

func TestTicketOptimisticLocking(t *testing.T) {
	server, _ := newTicketTestServer(t)
	admin := registerAndLogin(t, server, "lock-admin@example.com")

	typeID := createApprovalType(t, server, admin, "lock", "superadmin")
	created := createTicket(t, server, admin, typeID)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("create ticket: want non-empty id")
	}

	etag := getETag(t, server, "/api/tickets/"+id, admin)
	if etag == "" {
		t.Fatal("GET /api/tickets/:id must carry an ETag for If-Match clients")
	}

	// Matching tag: the edit applies.
	status, raw := requestHeaders(t, server, http.MethodPatch, "/api/tickets/"+id,
		`{"title":"Edited once","priority":"high"}`, admin, map[string]string{"If-Match": etag})
	if status != http.StatusOK {
		t.Fatalf("patch with current tag: want 200, got %d (%s)", status, raw)
	}

	// The same tag is now stale: 412, and the store keeps the first edit.
	status, raw = requestHeaders(t, server, http.MethodPatch, "/api/tickets/"+id,
		`{"title":"Clobbered","priority":"low"}`, admin, map[string]string{"If-Match": etag})
	if status != http.StatusPreconditionFailed {
		t.Fatalf("patch with stale tag: want 412, got %d (%s)", status, raw)
	}
	fresh, body := callJSON(t, server, http.MethodGet, "/api/tickets/"+id, "", admin)
	if fresh != http.StatusOK {
		t.Fatalf("get after stale patch: %d", fresh)
	}
	if ticket, _ := body["ticket"].(map[string]any); ticket != nil {
		if title, _ := ticket["title"].(string); title != "Edited once" {
			t.Fatalf("stale patch overwrote a concurrent edit: %v", ticket)
		}
	}

	// No header: previous last-write-wins behavior.
	status, _ = requestHeaders(t, server, http.MethodPatch, "/api/tickets/"+id,
		`{"title":"Edited freely"}`, admin, nil)
	if status != http.StatusOK {
		t.Fatalf("patch without If-Match: want 200, got %d", status)
	}
}

func TestUserOptimisticLocking(t *testing.T) {
	server := newTestServer(t)
	admin := registerAndLogin(t, server, "user-lock@example.com")

	_, me := callJSON(t, server, http.MethodGet, "/api/me", "", admin)
	id, _ := me["id"].(string)
	if id == "" {
		t.Fatal("/api/me: want non-empty id")
	}

	etag := getETag(t, server, "/api/users/"+id, admin)
	if etag == "" {
		t.Fatal("GET /api/users/:id must carry an ETag for If-Match clients")
	}

	status, _ := requestHeaders(t, server, http.MethodPut, "/api/users/"+id,
		`{"email":"user-lock@example.com","name":"Renamed Once","external_ref":""}`, admin,
		map[string]string{"If-Match": etag})
	if status != http.StatusOK {
		t.Fatalf("put with current tag: want 200, got %d", status)
	}

	status, raw := requestHeaders(t, server, http.MethodPut, "/api/users/"+id,
		`{"email":"user-lock@example.com","name":"Clobbered","external_ref":""}`, admin,
		map[string]string{"If-Match": etag})
	if status != http.StatusPreconditionFailed {
		t.Fatalf("put with stale tag: want 412, got %d (%s)", status, raw)
	}
	_, body := callJSON(t, server, http.MethodGet, "/api/users/"+id, "", admin)
	if name, _ := body["name"].(string); name != "Renamed Once" {
		t.Fatalf("stale put overwrote a concurrent edit: %v", body)
	}
}
