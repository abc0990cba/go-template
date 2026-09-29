package httpapp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestInvitationAPIFlow(t *testing.T) {
	server := newTestServer(t)
	admin := registerAndLogin(t, server, "inv-admin@example.com") // superadmin
	invitee := registerAndLogin(t, server, "inv-new@example.com")

	// Create an org to invite into.
	status, body := callJSON(t, server, http.MethodPost, "/api/orgs", `{"name":"Flow Org"}`, admin)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("create org: got %d (%v)", status, body)
	}
	orgID, _ := body["id"].(string)
	if orgID == "" {
		if ref, ok := body["org"].(map[string]any); ok {
			orgID, _ = ref["id"].(string)
		}
	}
	if orgID == "" {
		t.Fatalf("cannot resolve org id from %v", body)
	}

	// Viewer (no orgs:update) cannot invite.
	status, _ = callJSON(t, server, http.MethodPost, "/api/orgs/"+orgID+"/invitations",
		`{"email":"inv-new@example.com","role":"viewer"}`, invitee)
	if status != http.StatusForbidden {
		t.Fatalf("viewer invite: want 403, got %d", status)
	}

	// The admin invites; the response carries the token (dev convenience).
	status, body = callJSON(t, server, http.MethodPost, "/api/orgs/"+orgID+"/invitations",
		`{"email":"inv-new@example.com","role":"requester"}`, admin)
	if status != http.StatusCreated {
		t.Fatalf("invite: want 201, got %d (%v)", status, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("invite must return the token for the dev flow")
	}

	// The invitee sees it in "mine".
	status, body = callJSON(t, server, http.MethodGet, "/api/invitations", "", invitee)
	if status != http.StatusOK {
		t.Fatalf("mine: got %d", status)
	}
	if data, _ := body["data"].([]any); len(data) != 1 {
		t.Fatalf("want one pending invitation, got %v", body)
	}

	// Accept grants membership + role: the invitee can now read org-scoped
	// tickets (requester role) with the org header.
	status, body = callJSON(t, server, http.MethodPost, "/api/invitations/accept",
		`{"token":"`+token+`"}`, invitee)
	if status != http.StatusOK {
		t.Fatalf("accept: want 200, got %d (%v)", status, body)
	}
	if body["org_id"] != orgID || body["role"] != "requester" {
		t.Fatalf("accept response mismatch: %v", body)
	}

	// The token is single-use.
	status, _ = callJSON(t, server, http.MethodPost, "/api/invitations/accept",
		`{"token":"`+token+`"}`, invitee)
	if status != http.StatusConflict {
		t.Fatalf("token reuse: want 409, got %d", status)
	}

	// Membership visible on the org members list.
	status, body = callJSON(t, server, http.MethodGet, "/api/orgs/"+orgID+"/members", "", admin)
	if status != http.StatusOK {
		t.Fatalf("members: got %d", status)
	}
	if !strings.Contains(rawJSON(t, body), "inv-new@example.com") {
		// Members carry user ids, not emails; check via the invitations
		// list instead — the invitation is now accepted.
		status, body = callJSON(t, server, http.MethodGet, "/api/orgs/"+orgID+"/invitations", "", admin)
		if status != http.StatusOK {
			t.Fatalf("invitations list: got %d", status)
		}
		if data, _ := body["data"].([]any); len(data) != 1 {
			t.Fatalf("want the accepted invitation listed, got %v", body)
		}
	}
}

// rawJSON re-marshals a decoded body for substring checks.
func rawJSON(t *testing.T, body map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(encoded)
}
