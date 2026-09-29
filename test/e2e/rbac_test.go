//go:build e2e

// RBAC end-to-end: a superadmin creates a custom role, assigns it to a
// fresh account, and the account observes its narrowed access (403 where
// the permission is missing, 200 where it is granted). Requires the live
// stack (make docker-up); skips itself otherwise.
package e2e

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestRBACEnforcement(t *testing.T) {
	client := requireStack(t)

	adminToken := registerUnique(t, client)
	targetToken, targetID := registerUniqueWithID(t, client)

	// The suite's "admin" is only a superadmin on a fresh database: the
	// register flow grants superadmin to the first account. Seeded stacks
	// (make docker-up with APP_DEMO_SEED) ship their own users, so skip
	// instead of failing on an environment property.
	_, me := call(t, client, http.MethodGet, "/api/me", "", adminToken)
	roles, _ := me["roles"].([]any)
	isSuper := false
	for _, role := range roles {
		if role == "superadmin" {
			isSuper = true
		}
	}
	if !isSuper {
		t.Skip("registered account is not superadmin — the stack database is seeded; run against a fresh database")
	}

	// The fresh account has viewer rights only: reads pass, writes fail.
	status, _ := call(t, client, http.MethodGet, "/api/users?limit=1", "", targetToken)
	if status != http.StatusForbidden {
		t.Fatalf("viewer list users: want 403, got %d", status)
	}
	status, _ = call(t, client, http.MethodGet, "/api/analytics/summary", "", targetToken)
	if status != http.StatusOK {
		t.Fatalf("viewer analytics: want 200, got %d", status)
	}

	// Superadmin creates a role granting users:read and assigns it.
	_, roleBody := call(t, client, http.MethodPost, "/api/roles",
		`{"key":"e2e-reader","name":"E2E Reader","description":"created by the e2e suite"}`, adminToken)
	if roleBody["key"] != "e2e-reader" && roleBody["id"] == nil {
		t.Fatalf("create role: %v", roleBody)
	}
	status, body := call(t, client, http.MethodPut, "/api/roles/e2e-reader/permissions",
		`{"domain":"*","permissions":[{"object":"users","action":"read"}]}`, adminToken)
	if status != http.StatusNoContent {
		t.Fatalf("set role permissions: %d %v", status, body)
	}
	status, _ = call(t, client, http.MethodPost, "/api/users/"+targetID+"/roles",
		`{"role":"e2e-reader","domain":"*"}`, adminToken)
	if status != http.StatusNoContent {
		t.Fatalf("assign role: %d", status)
	}

	// The grant takes effect: reads now pass, writes still fail.
	status, _ = call(t, client, http.MethodGet, "/api/users?limit=1", "", targetToken)
	if status != http.StatusOK {
		t.Fatalf("granted list users: want 200, got %d", status)
	}
	status, _ = call(t, client, http.MethodPost, "/api/users",
		`{"email":"denied@example.com","name":"Nope"}`, targetToken)
	if status != http.StatusForbidden {
		t.Fatalf("granted create user: want 403, got %d", status)
	}

	// The raw policy editor stays closed for non-superadmins.
	status, _ = call(t, client, http.MethodGet, "/api/policies?ptype=p", "", targetToken)
	if status != http.StatusForbidden {
		t.Fatalf("viewer raw policies: want 403, got %d", status)
	}

	// Cleanup: revoke and delete the role.
	_, _ = call(t, client, http.MethodDelete, "/api/users/"+targetID+"/roles",
		`{"role":"e2e-reader","domain":"*"}`, adminToken)
	status, _ = call(t, client, http.MethodDelete, "/api/roles/e2e-reader", "", adminToken)
	if status != http.StatusNoContent {
		t.Fatalf("delete role: %d", status)
	}
}

func registerUnique(t *testing.T, client *http.Client) string {
	t.Helper()
	token, _ := registerUniqueWithID(t, client)
	return token
}

func registerUniqueWithID(t *testing.T, client *http.Client) (string, string) {
	t.Helper()
	email := fmt.Sprintf("rbac-e2e-%d@example.com", time.Now().UnixNano())
	status, body := call(t, client, http.MethodPost, "/api/auth/register",
		`{"email":"`+email+`","name":"RBAC E2E","password":"password123"}`, "")
	if status != http.StatusCreated {
		t.Fatalf("register %s: %d %v", email, status, body)
	}
	token, _ := body["token"].(string)
	user, _ := body["user"].(map[string]any)
	id, _ := user["id"].(string)
	if token == "" || id == "" {
		t.Fatalf("register %s: missing token or id: %v", email, body)
	}
	return token, id
}
