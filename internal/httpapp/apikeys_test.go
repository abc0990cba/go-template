package httpapp

import (
	"net/http"
	"strings"
	"testing"
)

// TestAPIKeyScopesNarrowPermissions proves the guard rails: a key never
// widens its owner's access, but it also never exceeds its own scopes —
// even when the owner is a superadmin.
func TestAPIKeyScopesNarrowPermissions(t *testing.T) {
	server := newTestServer(t)
	ownerToken := registerAndLogin(t, server, "owner@example.com") // first user: superadmin

	issue := func(t *testing.T, scopes string) string {
		t.Helper()
		status, body := callJSON(t, server, http.MethodPost, "/api/api-keys",
			`{"name":"ci","scopes":[`+scopes+`]}`, ownerToken)
		if status != http.StatusCreated {
			t.Fatalf("create key: want 201, got %d (%v)", status, body)
		}
		secret, _ := body["secret"].(string)
		return secret
	}

	getUsersWith := func(secret string) int {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/users", nil)
		req.Header.Set("Authorization", "Bearer "+secret)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("list users with key: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	// users:read covers the read but not the create, even though the owner
	// could do both.
	readKey := issue(t, `"users:read"`)
	if status := getUsersWith(readKey); status != http.StatusOK {
		t.Errorf("users:read key on GET /users: want 200, got %d", status)
	}
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/users",
		strings.NewReader(`{"email":"via-key@example.com","name":"Key"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+readKey)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("create user with read-only key: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("users:read key on POST /users: want 403, got %d", resp.StatusCode)
	}

	// An unrelated scope does not open the route at all.
	jobsKey := issue(t, `"jobs:execute"`)
	if status := getUsersWith(jobsKey); status != http.StatusForbidden {
		t.Errorf("jobs:execute key on GET /users: want 403, got %d", status)
	}

	// The object wildcard covers every action on the object.
	wildKey := issue(t, `"users:*"`)
	if status := getUsersWith(wildKey); status != http.StatusOK {
		t.Errorf("users:* key on GET /users: want 200, got %d", status)
	}

	// The owner's own session is untouched by key scopes: the same create
	// the key was denied succeeds.
	status, _ := callJSON(t, server, http.MethodPost, "/api/users",
		`{"email":"via-session@example.com","name":"Session"}`, ownerToken)
	if status != http.StatusCreated {
		t.Errorf("session create: want 201, got %d", status)
	}
}

// TestAPIKeyAuthRoundTrip issues a personal key over HTTP, authenticates
// with it, and proves revocation cuts access immediately.
func TestAPIKeyAuthRoundTrip(t *testing.T) {
	server := newTestServer(t)
	ownerToken := registerAndLogin(t, server, "owner@example.com")

	// A bad TTL is rejected before anything is minted.
	status, _ := callJSON(t, server, http.MethodPost, "/api/api-keys",
		`{"name":"ci","scopes":["users:read"],"ttl":"soon"}`, ownerToken)
	if status != http.StatusBadRequest {
		t.Errorf("bad ttl: want 400, got %d", status)
	}

	// Create: the raw secret is returned exactly once.
	status, body := callJSON(t, server, http.MethodPost, "/api/api-keys",
		`{"name":"ci","scopes":["users:read"],"ttl":"24h"}`, ownerToken)
	if status != http.StatusCreated {
		t.Fatalf("create key: want 201, got %d (%v)", status, body)
	}
	secret, _ := body["secret"].(string)
	if !strings.HasPrefix(secret, "gtpl_") {
		t.Fatalf("create key: want gtpl_ secret, got %q", secret)
	}
	key, _ := body["key"].(map[string]any)
	keyID, _ := key["id"].(string)
	if keyID == "" {
		t.Fatalf("create key: want key id, got %v", body)
	}

	// The key authenticates without a session cookie, as its owner.
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("me with api key: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("me with api key: want 200, got %d", resp.StatusCode)
	}

	// A wrong or unknown secret is 401.
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
	req.Header.Set("Authorization", "Bearer gtpl_definitely-not-a-key")
	resp, err = server.Client().Do(req)
	if err != nil {
		t.Fatalf("me with bad key: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("me with bad key: want 401, got %d", resp.StatusCode)
	}

	// The listing shows the prefix and scopes but never the secret.
	status, body = callJSON(t, server, http.MethodGet, "/api/api-keys", "", ownerToken)
	if status != http.StatusOK {
		t.Fatalf("list keys: want 200, got %d", status)
	}
	data, _ := body["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("list keys: want 1, got %d (%v)", len(data), body)
	}
	listed, _ := data[0].(map[string]any)
	if listed["prefix"] == "" || listed["prefix"] == secret {
		t.Errorf("list keys: want display prefix only, got %v", listed["prefix"])
	}

	// Revocation cuts access on the next request.
	status, _ = callJSON(t, server, http.MethodDelete, "/api/api-keys/"+keyID, "", ownerToken)
	if status != http.StatusNoContent {
		t.Fatalf("revoke: want 204, got %d", status)
	}
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err = server.Client().Do(req)
	if err != nil {
		t.Fatalf("me with revoked key: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("me with revoked key: want 401, got %d", resp.StatusCode)
	}
}
