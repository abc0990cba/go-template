package httpapp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/repository"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	logger := discardLogger()
	store := repository.NewMemoryStore()
	services := newTestContainer(t, store, nil, logger)
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}, realtime.NewHub()))
	t.Cleanup(server.Close)
	return server
}

// registerAndLogin creates a fresh account and returns its bearer token.
func registerAndLogin(t *testing.T, server *httptest.Server, email string) string {
	t.Helper()
	status, body := callJSON(t, server, http.MethodPost, "/api/auth/register",
		`{"email":"`+email+`","name":"Ada","password":"password123"}`, "")
	if status != http.StatusCreated {
		t.Fatalf("register %s: want 201, got %d (%v)", email, status, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("register: want non-empty token")
	}
	return token
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
	token := registerAndLogin(t, server, "owner@example.com")

	status, _ := callJSON(t, server, http.MethodPost, "/api/users", `{"email":"no-email"}`, token)
	if status != http.StatusBadRequest {
		t.Errorf("create user with invalid email: want 400, got %d", status)
	}

	status, _ = callJSON(t, server, http.MethodPost, "/api/users", `{"email":"ada@example.com","name":"Ada"}`, token)
	if status != http.StatusCreated {
		t.Fatalf("create user: want 201, got %d", status)
	}
	status, _ = callJSON(t, server, http.MethodPost, "/api/users", `{"email":"ada@example.com","name":"Ada"}`, token)
	if status != http.StatusConflict {
		t.Errorf("duplicate user: want 409, got %d", status)
	}

	status, body := callJSON(t, server, http.MethodGet, "/api/users?limit=1", "", token)
	if status != http.StatusOK {
		t.Fatalf("list users: want 200, got %d", status)
	}
	data, _ := body["data"].([]any)
	if len(data) != 1 {
		t.Errorf("list users: want 1 entry, got %d (%v)", len(data), body)
	}

	// The API no longer answers anonymous data requests.
	status, _ = callJSON(t, server, http.MethodGet, "/api/users", "", "")
	if status != http.StatusUnauthorized {
		t.Errorf("list users without token: want 401, got %d", status)
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

func TestMetricsAndVersionEndpoints(t *testing.T) {
	server := newTestServer(t)

	// Warm at least one request so RED counters are non-empty.
	resp, err := server.Client().Get(server.URL + "/health/live")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	_ = resp.Body.Close()

	resp, err = server.Client().Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics: want 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	for _, want := range []string{
		"app_http_requests_total",
		"app_http_request_duration_seconds",
		"app_http_requests_in_flight",
		"app_jobs_queued_total",
		"go_goroutines",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("metrics: missing %s", want)
		}
	}

	versionResp, err := server.Client().Get(server.URL + "/version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	defer func() { _ = versionResp.Body.Close() }()
	var parsed map[string]any
	if err := json.NewDecoder(versionResp.Body).Decode(&parsed); err != nil {
		t.Fatalf("version decode: %v", err)
	}
	if parsed["version"] != "dev" {
		t.Errorf("version: want dev, got %v", parsed["version"])
	}
}

func TestUserManagementAndJobEndpoints(t *testing.T) {
	server := newTestServer(t)
	token := registerAndLogin(t, server, "ada@example.com")
	// The caller's own account, used for identity-sensitive checks below.
	_, body := callJSON(t, server, http.MethodGet, "/api/me", "", token)

	// GET/PUT/DELETE by id, against a managed user (deleting the caller's
	// own account would cascade the session away with it).
	status, managed := callJSON(t, server, http.MethodPost, "/api/users",
		`{"email":"managed@example.com","name":"Managed"}`, token)
	if status != http.StatusCreated {
		t.Fatalf("create managed user: %d %v", status, managed)
	}
	managedID, _ := managed["id"].(string)
	status, body = callJSON(t, server, http.MethodGet, "/api/users/"+managedID, "", token)
	if status != http.StatusOK || body["id"] != managedID {
		t.Errorf("get user: %d %v", status, body)
	}
	status, body = callJSON(t, server, http.MethodPut, "/api/users/"+managedID,
		`{"email":"renamed@example.com","name":"Renamed"}`, token)
	if status != http.StatusOK || body["name"] != "Renamed" {
		t.Errorf("update user: %d %v", status, body)
	}
	status, _ = callJSON(t, server, http.MethodDelete, "/api/users/"+managedID, "", token)
	if status != http.StatusNoContent {
		t.Errorf("delete user: want 204, got %d", status)
	}
	status, _ = callJSON(t, server, http.MethodGet, "/api/users/"+managedID, "", token)
	if status != http.StatusNotFound {
		t.Errorf("get deleted user: want 404, got %d", status)
	}

	// Jobs: enqueue enrichment and a report; both accepted and listed.
	status, _ = callJSON(t, server, http.MethodPost, "/api/jobs/enrichment",
		`{"count":3}`, token)
	if status != http.StatusAccepted {
		t.Errorf("enqueue enrichment: want 202, got %d", status)
	}
	status, _ = callJSON(t, server, http.MethodPost, "/api/reports", "{}", token)
	if status != http.StatusAccepted {
		t.Errorf("generate report: want 202, got %d", status)
	}
	status, body = callJSON(t, server, http.MethodGet, "/api/jobs", "", token)
	if status != http.StatusOK {
		t.Fatalf("list jobs: want 200, got %d", status)
	}
	if data, _ := body["data"].([]any); len(data) != 2 {
		t.Errorf("jobs: want 2, got %v", body)
	}

	// Missing user yields 404 problem+json.
	status, body = callJSON(t, server, http.MethodGet, "/api/users/nope", "", token)
	if status != http.StatusNotFound {
		t.Errorf("missing user: want 404, got %d", status)
	}
	if body["title"] != "not found" {
		t.Errorf("problem title: want not found, got %v", body["title"])
	}
}

func TestPaginationModesAcrossCollections(t *testing.T) {
	server := newTestServer(t)
	token := registerAndLogin(t, server, "pager@example.com")

	// A deterministic population: the registering user plus 7 managed ones,
	// created in ascending order.
	ids := make([]string, 0, 8)
	_, me := callJSON(t, server, http.MethodGet, "/api/me", "", token)
	ids = append(ids, me["id"].(string))
	for i := 1; i <= 7; i++ {
		email := "page-" + strings.Repeat("x", 20) + strconv.Itoa(i) + "@example.com"
		status, body := callJSON(t, server, http.MethodPost, "/api/users",
			`{"email":"`+email+`","name":"Page `+strconv.Itoa(i)+`"}`, token)
		if status != http.StatusCreated {
			t.Fatalf("seed user %d: %d", i, status)
		}
		ids = append(ids, body["id"].(string))
	}

	for _, mode := range []string{"offset", "cursor", "keyset"} {
		seen := 0
		after := ""
		pages := 0
		for seen < 8 && pages < 10 {
			path := "/api/users?mode=" + mode + "&limit=3"
			if after != "" {
				path += "&after=" + after
				if mode == "offset" {
					path = "/api/users?mode=offset&limit=3&offset=" + strconv.Itoa(seen)
				}
			}
			status, body := callJSON(t, server, http.MethodGet, path, "", token)
			if status != http.StatusOK {
				t.Fatalf("%s page %d: %d", mode, pages, status)
			}
			data, _ := body["data"].([]any)
			if len(data) == 0 {
				break
			}
			seen += len(data)
			pages++
			if next, _ := body["next_cursor"].(string); next != "" {
				after = next
			} else {
				break
			}
		}
		if seen != 8 {
			t.Errorf("mode %s: want to page through 8 users, saw %d", mode, seen)
		}
	}

	// Job and report listings answer in every mode too.
	for _, path := range []string{"/api/jobs", "/api/reports", "/api/enrichment-profiles"} {
		for _, mode := range []string{"offset", "cursor", "keyset"} {
			status, _ := callJSON(t, server, http.MethodGet, path+"?mode="+mode+"&limit=1", "", token)
			if status != http.StatusOK {
				t.Errorf("%s mode %s: want 200, got %d", path, mode, status)
			}
		}
	}
}

func TestAnalyticsAndSessionEndpoints(t *testing.T) {
	server := newTestServer(t)
	token := registerAndLogin(t, server, "analytics@example.com")

	status, body := callJSON(t, server, http.MethodGet, "/api/analytics/summary", "", token)
	if status != http.StatusOK || body["total_users"] == nil {
		t.Errorf("summary: %d %v", status, body)
	}
	status, _ = callJSON(t, server, http.MethodGet, "/api/analytics/timeseries?days=7", "", token)
	if status != http.StatusOK {
		t.Errorf("timeseries: want 200, got %d", status)
	}
	status, _ = callJSON(t, server, http.MethodGet, "/api/analytics/events?limit=5", "", token)
	if status != http.StatusOK {
		t.Errorf("recent events: want 200, got %d", status)
	}

	status, body = callJSON(t, server, http.MethodGet, "/api/sessions", "", token)
	if status != http.StatusOK {
		t.Fatalf("sessions: want 200, got %d", status)
	}
	if data, _ := body["data"].([]any); len(data) != 1 {
		t.Errorf("sessions: want exactly the current session, got %v", body)
	}

	status, body = callJSON(t, server, http.MethodGet, "/api/oauth/providers", "", "")
	if status != http.StatusOK {
		t.Fatalf("oauth providers: want 200, got %d", status)
	}
	if data, _ := body["data"].([]any); len(data) != 2 {
		t.Errorf("oauth providers: want 2 entries, got %v", body)
	}
}
