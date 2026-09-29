//go:build e2e

// Command e2e runs the API happy path against a live stack (make docker-up):
// register → login → me → create/list users → enqueue jobs → poll until done.
// The suite skips itself when the stack is unreachable, so a plain
// `go test ./...` never fails because the environment is down.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

// Overridable so the suite can point at a non-default port (tests, shared hosts).
var baseURL = envOr("GO_TEMPLATE_BASE_URL", "http://localhost:8080")

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requireStack(t *testing.T) *http.Client {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(baseURL + "/health/ready")
	if err != nil {
		t.Skipf("stack not reachable at %s: %v (run `make docker-up`)", baseURL, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("stack not ready: health %d", resp.StatusCode)
	}
	return client
}

func call(t *testing.T, client *http.Client, method, path, body, token string) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
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
		t.Fatalf("%s %s: decode: %v", method, path, err)
	}
	return resp.StatusCode, parsed
}

func TestAPIHappyPath(t *testing.T) {
	client := requireStack(t)

	// Register (unique email so reruns do not collide).
	email := fmt.Sprintf("e2e-%d@example.com", time.Now().UnixNano())
	status, body := call(t, client, http.MethodPost, "/api/auth/register",
		fmt.Sprintf(`{"email":%q,"name":"E2E","password":"password123"}`, email), "")
	if status != http.StatusCreated {
		t.Fatalf("register: want 201, got %d (%v)", status, body)
	}
	token, _ := body["token"].(string)

	// Login and identity.
	if status, _ = call(t, client, http.MethodPost, "/api/auth/login",
		fmt.Sprintf(`{"email":%q,"password":"password123"}`, email), ""); status != http.StatusOK {
		t.Fatalf("login: want 200, got %d", status)
	}
	if status, body = call(t, client, http.MethodGet, "/api/me", "", token); status != http.StatusOK {
		t.Fatalf("me: want 200, got %d", status)
	}

	// Users: create then list (a distinct email: emails are unique).
	if status, _ = call(t, client, http.MethodPost, "/api/users",
		fmt.Sprintf(`{"email":%q,"name":"Managed"}`, "managed-"+email), token); status != http.StatusCreated {
		t.Fatalf("create user: want 201, got %d", status)
	}
	if status, body = call(t, client, http.MethodGet, "/api/users?limit=5", "", token); status != http.StatusOK {
		t.Fatalf("list users: want 200, got %d", status)
	}
	if data, _ := body["data"].([]any); len(data) < 1 {
		t.Errorf("users page empty: %v", body)
	}

	// Jobs: enqueue a report and wait for it to reach done.
	if status, _ = call(t, client, http.MethodPost, "/api/reports", "{}", token); status != http.StatusAccepted {
		t.Fatalf("generate report: want 202, got %d", status)
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		var done bool
		if status, body = call(t, client, http.MethodGet, "/api/jobs", "", token); status == http.StatusOK {
			if data, _ := body["data"].([]any); len(data) > 0 {
				job, _ := data[0].(map[string]any)
				switch job["status"] {
				case "done":
					done = true
				case "failed":
					t.Fatalf("report job failed: %v", job["error"])
				}
			}
		}
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("report job did not finish in time")
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Analytics and reports endpoints answer for the new state.
	if status, _ = call(t, client, http.MethodGet, "/api/analytics/summary", "", token); status != http.StatusOK {
		t.Errorf("analytics: want 200, got %d", status)
	}
	if status, _ = call(t, client, http.MethodGet, "/api/reports", "", token); status != http.StatusOK {
		t.Errorf("reports: want 200, got %d", status)
	}
}

// TestTicketApprovalFlow exercises the tickets vertical against the live
// stack: the superadmin demo account files a ticket, submits, and approves
// it through the admin bypass.
func TestTicketApprovalFlow(t *testing.T) {
	client := requireStack(t)

	email := os.Getenv("APP_DEMO_EMAIL")
	password := os.Getenv("APP_DEMO_PASSWORD")
	if email == "" || password == "" {
		t.Skip("demo credentials not configured (APP_DEMO_EMAIL / APP_DEMO_PASSWORD)")
	}
	if status, body := call(t, client, http.MethodPost, "/api/auth/login",
		fmt.Sprintf(`{"email":%q,"password":%q}`, email, password), ""); status != http.StatusOK {
		t.Skipf("demo login failed: %d (%v)", status, body)
	} else if token, _ := body["token"].(string); token == "" {
		t.Fatal("login: want token")
	} else {
		// Rebind token for the rest of the test.
		client = tokenClient(token)
	}

	// A ticket type with one step decided by whoever holds the approver
	// role; the admin acts on the step through the org-admin bypass.
	status, typ := call(t, client, http.MethodPost, "/api/ticket-types",
		`{"key":"e2e-flow","name":"E2E flow","steps":[{"name":"Review","approver_role":"approver"}]}`, "")
	if status == http.StatusConflict {
		// Rerun: resolve the existing type through the catalog.
		if status, types := call(t, client, http.MethodGet, "/api/ticket-types", "", ""); status != http.StatusOK {
			t.Fatalf("list ticket types: want 200, got %d", status)
		} else {
			typ = findMap(types, "key", "e2e-flow")
		}
	} else if status != http.StatusCreated {
		t.Fatalf("create ticket type: want 201, got %d (%v)", status, typ)
	}
	typeID, _ := typ["id"].(string)

	status, ticket := call(t, client, http.MethodPost, "/api/tickets",
		fmt.Sprintf(`{"type_id":%q,"title":"E2E approval flow","priority":"medium"}`, typeID), "")
	if status != http.StatusCreated {
		t.Fatalf("create ticket: want 201, got %d (%v)", status, ticket)
	}
	ticketID, _ := ticket["id"].(string)

	if status, _ = call(t, client, http.MethodPost, "/api/tickets/"+ticketID+"/actions",
		`{"action":"submit"}`, ""); status != http.StatusOK {
		t.Fatalf("submit: want 200")
	}
	if status, inbox := call(t, client, http.MethodGet, "/api/approvals", "", ""); status != http.StatusOK {
		t.Errorf("approvals: want 200, got %d", status)
	} else if findMap(inbox, "id", ticketID) == nil {
		t.Errorf("approvals inbox: ticket %s not queued", ticketID)
	}
	if status, after := call(t, client, http.MethodPost, "/api/tickets/"+ticketID+"/actions",
		`{"action":"approve","comment":"verified"}`, ""); status != http.StatusOK || after["status"] != "approved" {
		t.Fatalf("approve: want 200 approved, got %d (%v)", status, after["status"])
	}

	if status, _ = call(t, client, http.MethodPost, "/api/tickets/"+ticketID+"/comments",
		`{"body":"Flow verified"}`, ""); status != http.StatusCreated {
		t.Errorf("comment: want 201, got %d", status)
	}
	if status, _ = call(t, client, http.MethodGet, "/api/tickets/"+ticketID+"/events", "", ""); status != http.StatusOK {
		t.Errorf("events: want 200")
	}
}

// tokenClient wraps an http.Client adding the bearer token to every request.
func tokenClient(token string) *http.Client {
	return &http.Client{Transport: bearerTransport{token: token}}
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(req)
}

// findMap finds the element of the "data" array whose key matches want.
func findMap(parsed map[string]any, key, want string) map[string]any {
	data, _ := parsed["data"].([]any)
	for _, item := range data {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if value, _ := m[key].(string); value == want {
			return m
		}
	}
	return nil
}

func TestHealthAndVersion(t *testing.T) {
	client := requireStack(t)
	resp, err := client.Get(baseURL + "/version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("version: want 200, got %d", resp.StatusCode)
	}
	if os.Getenv("E2E_STRICT") != "" {
		if resp.Header.Get("X-Request-ID") == "" {
			t.Error("X-Request-ID missing on live response")
		}
	}
}
