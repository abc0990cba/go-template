package httpapp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

// registerUser creates a fresh account and returns its id and token. The
// first account on a fresh store is superadmin, every later one a viewer.
func registerAccount(t *testing.T, server *httptest.Server, email string) (string, string) {
	t.Helper()
	status, body := callJSON(t, server, http.MethodPost, "/api/auth/register",
		`{"email":"`+email+`","name":"Ada","password":"password123"}`, "")
	if status != http.StatusCreated {
		t.Fatalf("register %s: want 201, got %d (%v)", email, status, body)
	}
	user, _ := body["user"].(map[string]any)
	id, _ := user["id"].(string)
	token, _ := body["token"].(string)
	if id == "" || token == "" {
		t.Fatalf("register %s: want user id and token, got %v", email, body)
	}
	return id, token
}

// postWith sends one JSON request with an explicit client (the cookie-jar
// browser simulations need their own client, not the shared server one).
func postWith(t *testing.T, client *http.Client, server *httptest.Server, method, path, body, token string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, server.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var parsed map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil && err.Error() != "EOF" {
		t.Fatalf("%s %s: decode response: %v", method, path, err)
	}
	return resp.StatusCode, parsed
}

// TestImpersonationFlow covers the admin guard rails and the full
// impersonate → act-as → stop round trip, including the audit trail and
// the target's notification.
func TestImpersonationFlow(t *testing.T) {
	server := newTestServer(t)
	adminID, adminToken := registerAccount(t, server, "admin@example.com")
	targetID, targetToken := registerAccount(t, server, "target@example.com")

	// A viewer lacks impersonation:execute.
	status, _ := callJSON(t, server, http.MethodPost, "/api/impersonation",
		`{"user_id":"`+adminID+`"}`, targetToken)
	if status != http.StatusForbidden {
		t.Errorf("viewer impersonation: want 403, got %d", status)
	}

	// An unknown target is 404, not a silent success.
	status, _ = callJSON(t, server, http.MethodPost, "/api/impersonation",
		`{"user_id":"ghost"}`, adminToken)
	if status != http.StatusNotFound {
		t.Errorf("unknown target: want 404, got %d", status)
	}

	// Impersonating yourself is rejected.
	status, _ = callJSON(t, server, http.MethodPost, "/api/impersonation",
		`{"user_id":"`+adminID+`"}`, adminToken)
	if status != http.StatusBadRequest {
		t.Errorf("self impersonation: want 400, got %d", status)
	}

	// A browser client keeps the session in its cookie jar: login, start
	// the impersonation, then continue cookie-only like the SPA does.
	jar, _ := cookiejar.New(nil)
	browser := *server.Client()
	browser.Jar = jar

	status, _ = postWith(t, &browser, server, http.MethodPost, "/api/auth/login",
		`{"email":"admin@example.com","password":"password123"}`, "")
	if status != http.StatusOK {
		t.Fatalf("admin login: want 200, got %d", status)
	}

	status, body := postWith(t, &browser, server, http.MethodPost, "/api/impersonation",
		`{"user_id":"`+targetID+`"}`, "")
	if status != http.StatusOK {
		t.Fatalf("impersonate: want 200, got %d (%v)", status, body)
	}
	if body["status"] != "impersonating" || body["user_id"] != targetID {
		t.Errorf("impersonate body: want impersonating/%s, got %v", targetID, body)
	}

	// The cookie now carries the impersonated session: /api/me answers as
	// the target and flags the impersonator.
	status, me := postWith(t, &browser, server, http.MethodGet, "/api/me", "", "")
	if status != http.StatusOK {
		t.Fatalf("me while impersonating: want 200, got %d", status)
	}
	if me["email"] != "target@example.com" {
		t.Errorf("me email: want target@example.com, got %v", me["email"])
	}
	if me["impersonated"] != true || me["impersonator_name"] != "admin@example.com" {
		t.Errorf("me impersonation flags: want true/admin@example.com, got %v/%v",
			me["impersonated"], me["impersonator_name"])
	}

	// The target is told about the access.
	status, notifications := callJSON(t, server, http.MethodGet, "/api/notifications", "", targetToken)
	if status != http.StatusOK {
		t.Fatalf("target notifications: want 200, got %d", status)
	}
	if !listContains(notifications, "title", "An administrator signed in to your account") {
		t.Errorf("target notification missing: %v", notifications)
	}

	// The audit trail records the start.
	status, trail := callJSON(t, server, http.MethodGet, "/api/audit?action=impersonation.started", "", adminToken)
	if status != http.StatusOK {
		t.Fatalf("audit: want 200, got %d", status)
	}
	if !listContains(trail, "action", "impersonation.started") {
		t.Errorf("audit trail missing impersonation.started: %v", trail)
	}

	// Stopping restores the admin identity and ends the session.
	status, _ = postWith(t, &browser, server, http.MethodPost, "/api/impersonation/stop", "", "")
	if status != http.StatusNoContent {
		t.Fatalf("stop impersonation: want 204, got %d", status)
	}
	status, me = postWith(t, &browser, server, http.MethodGet, "/api/me", "", "")
	if status != http.StatusOK {
		t.Fatalf("me after stop: want 200, got %d", status)
	}
	if me["email"] != "admin@example.com" || me["impersonated"] != false {
		t.Errorf("me after stop: want admin/not impersonated, got %v/%v", me["email"], me["impersonated"])
	}

	// The restored session is a plain admin session: stopping again is 400.
	status, _ = postWith(t, &browser, server, http.MethodPost, "/api/impersonation/stop", "", "")
	if status != http.StatusBadRequest {
		t.Errorf("stop without impersonation: want 400, got %d", status)
	}
}

// listContains reports whether body.data holds one object whose key equals
// want (the shape every list endpoint answers with).
func listContains(body map[string]any, key, want string) bool {
	data, _ := body["data"].([]any)
	for _, entry := range data {
		if item, ok := entry.(map[string]any); ok && item[key] == want {
			return true
		}
	}
	return false
}
