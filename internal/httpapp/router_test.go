package httpapp

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	services := service.NewContainer(repository.NewMemoryStore(), logger, cfg)
	server := httptest.NewServer(NewRouter(cfg, services, logger))
	t.Cleanup(server.Close)
	return server
}

func callJSON(t *testing.T, server *httptest.Server, method, path, body, token string) (int, map[string]any) {
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
	resp, err := server.Client().Do(req)
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

func TestHealthEndpoints(t *testing.T) {
	server := newTestServer(t)
	for _, path := range []string{"/health/live", "/health/ready", "/health/startup"} {
		status, body := callJSON(t, server, http.MethodGet, path, "", "")
		if status != http.StatusOK {
			t.Errorf("%s: want 200, got %d", path, status)
		}
		if body["status"] != "ok" {
			t.Errorf("%s: want status ok, got %v", path, body)
		}
	}
}

func TestAuthFlowAndErrorMapping(t *testing.T) {
	server := newTestServer(t)

	status, body := callJSON(t, server, http.MethodPost, "/api/auth/register",
		`{"email":"ada@example.com","name":"Ada","password":"password123"}`, "")
	if status != http.StatusCreated {
		t.Fatalf("register: want 201, got %d (%v)", status, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("register: want non-empty token")
	}

	status, _ = callJSON(t, server, http.MethodPost, "/api/auth/register",
		`{"email":"ada@example.com","name":"Ada","password":"password123"}`, "")
	if status != http.StatusConflict {
		t.Errorf("duplicate register: want 409, got %d", status)
	}

	status, _ = callJSON(t, server, http.MethodPost, "/api/auth/register",
		`{"email":"bob@example.com","name":"Bob","password":"short"}`, "")
	if status != http.StatusBadRequest {
		t.Errorf("short password: want 400, got %d", status)
	}

	status, body = callJSON(t, server, http.MethodPost, "/api/auth/login",
		`{"email":"ada@example.com","password":"wrong-password"}`, "")
	if status != http.StatusUnauthorized {
		t.Errorf("bad login: want 401, got %d", status)
	}
	if body["title"] != "invalid credentials" {
		t.Errorf("bad login title: want invalid credentials, got %v", body["title"])
	}

	status, body = callJSON(t, server, http.MethodPost, "/api/auth/login",
		`{"email":"ada@example.com","password":"password123"}`, "")
	if status != http.StatusOK {
		t.Fatalf("login: want 200, got %d", status)
	}
	if token, _ = body["token"].(string); token == "" {
		t.Fatal("login: want non-empty token")
	}

	status, body = callJSON(t, server, http.MethodGet, "/api/me", "", token)
	if status != http.StatusOK {
		t.Fatalf("me: want 200, got %d", status)
	}
	if body["email"] != "ada@example.com" {
		t.Errorf("me: want ada@example.com, got %v", body)
	}

	status, _ = callJSON(t, server, http.MethodGet, "/api/me", "", "")
	if status != http.StatusUnauthorized {
		t.Errorf("me without token: want 401, got %d", status)
	}
}

func TestUserEndpoints(t *testing.T) {
	server := newTestServer(t)

	status, _ := callJSON(t, server, http.MethodPost, "/api/users", `{"email":"no-email"}`, "")
	if status != http.StatusBadRequest {
		t.Errorf("create user with invalid email: want 400, got %d", status)
	}

	status, _ = callJSON(t, server, http.MethodPost, "/api/users", `{"email":"ada@example.com","name":"Ada"}`, "")
	if status != http.StatusCreated {
		t.Fatalf("create user: want 201, got %d", status)
	}
	status, _ = callJSON(t, server, http.MethodPost, "/api/users", `{"email":"ada@example.com","name":"Ada"}`, "")
	if status != http.StatusConflict {
		t.Errorf("duplicate user: want 409, got %d", status)
	}

	status, body := callJSON(t, server, http.MethodGet, "/api/users?limit=1", "", "")
	if status != http.StatusOK {
		t.Fatalf("list users: want 200, got %d", status)
	}
	data, _ := body["data"].([]any)
	if len(data) != 1 {
		t.Errorf("list users: want 1 entry, got %d (%v)", len(data), body)
	}
}

func TestRequestIDHeaderSetOnResponses(t *testing.T) {
	server := newTestServer(t)
	req, err := http.NewRequest(http.MethodGet, server.URL+"/health/live", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("X-Request-ID", "upstream-42")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	_ = resp.Body.Close()
	if got := resp.Header.Get("X-Request-ID"); got != "upstream-42" {
		t.Errorf("X-Request-ID: want upstream-42, got %q", got)
	}

	resp, err = server.Client().Get(server.URL + "/health/live")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	_ = resp.Body.Close()
	if got := resp.Header.Get("X-Request-ID"); got == "" {
		t.Error("X-Request-ID: want generated id, got empty")
	}
}
