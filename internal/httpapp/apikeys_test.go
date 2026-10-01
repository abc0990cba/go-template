package httpapp

import (
	"net/http"
	"strings"
	"testing"
)

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
