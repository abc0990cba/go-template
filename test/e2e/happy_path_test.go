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

const baseURL = "http://localhost:8080"

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

	// Users: create then list.
	if status, _ = call(t, client, http.MethodPost, "/api/users",
		fmt.Sprintf(`{"email":%q,"name":"Managed"}`, email), token); status != http.StatusCreated {
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
	if status, _ = call(t, client, http.MethodGet, "/api/analytics/summary", "", ""); status != http.StatusOK {
		t.Errorf("analytics: want 200, got %d", status)
	}
	if status, _ = call(t, client, http.MethodGet, "/api/reports", "", token); status != http.StatusOK {
		t.Errorf("reports: want 200, got %d", status)
	}
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
