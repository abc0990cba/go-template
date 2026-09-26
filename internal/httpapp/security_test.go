package httpapp

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

// newSecurityServer builds the full router. The returned rebuild closure
// re-creates the handler after tests mutate cfg in ways that affect
// construction-time state (for example BaseURL).
func newSecurityServer(t *testing.T, mutate func(*config.Config)) (*httptest.Server, *config.Config, func()) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if mutate != nil {
		mutate(&cfg)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := repository.NewMemoryStore()
	services := service.NewContainer(store, store, store, store, store, nil, nil, nil, nil, logger, cfg)
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}))
	jar, _ := cookiejar.New(nil)
	server.Client().Jar = jar
	t.Cleanup(server.Close)
	rebuild := func() {
		server.Config.Handler = NewRouter(cfg, services, logger, store, observ.Build{})
	}
	return server, &cfg, rebuild
}

func TestCSRFProtectionOnHTMLForms(t *testing.T) {
	server, _, _ := newSecurityServer(t, nil)

	// Visiting the login page issues the cookie.
	page, err := server.Client().Get(server.URL + "/login")
	if err != nil {
		t.Fatalf("login page: %v", err)
	}
	_ = page.Body.Close()
	var csrf string
	for _, cookie := range page.Cookies() {
		if cookie.Name == "app_csrf" {
			csrf = cookie.Value
		}
	}
	if csrf == "" {
		t.Fatal("login page did not issue app_csrf cookie")
	}

	// Posting without the form token is rejected.
	resp, err := server.Client().PostForm(server.URL+"/login", url.Values{"email": {"a@b.c"}, "password": {"password1"}})
	if err != nil {
		t.Fatalf("post without csrf: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("post without csrf: want 403, got %d", resp.StatusCode)
	}

	// Posting with a mismatched token is rejected too.
	resp, err = server.Client().PostForm(server.URL+"/login", url.Values{
		"email": {"a@b.c"}, "password": {"password1"}, "csrf_token": {"forged"},
	})
	if err != nil {
		t.Fatalf("post with forged csrf: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("post with forged csrf: want 403, got %d", resp.StatusCode)
	}

	// The matching token passes.
	form := url.Values{"email": {"a@b.c"}, "password": {"password1"}, "csrf_token": {csrf}}
	resp, err = server.Client().PostForm(server.URL+"/login", form)
	if err != nil {
		t.Fatalf("post with csrf: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		t.Fatal("post with valid csrf was rejected")
	}

	// The JSON API does not require the token.
	status, _ := callJSONServer(t, server, http.MethodPost, "/api/auth/register",
		`{"email":"ada@example.com","password":"password123"}`, "")
	if status != http.StatusCreated {
		t.Fatalf("api register with csrf protection on: want 201, got %d", status)
	}
}

func TestSecurityHeadersPresent(t *testing.T) {
	server, _, _ := newSecurityServer(t, nil)
	resp, err := server.Client().Get(server.URL + "/health/live")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	_ = resp.Body.Close()
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	} {
		if got := resp.Header.Get(header); got != want {
			t.Errorf("%s: want %q, got %q", header, want, got)
		}
	}
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Error("Content-Security-Policy is missing")
	}
}

func TestAuthRateLimitReturns429(t *testing.T) {
	server, _, _ := newSecurityServer(t, func(cfg *config.Config) {
		cfg.AuthRateBurst = 3
		cfg.AuthRateRPS = 0.001
	})

	// The first burst passes; the next request from the same IP is denied.
	for i := 0; i < 3; i++ {
		resp, err := server.Client().PostForm(server.URL+"/login", url.Values{"email": {"a@b.c"}, "password": {"bad"}})
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("attempt %d hit the limit too early", i)
		}
	}
	resp, err := server.Client().PostForm(server.URL+"/login", url.Values{"email": {"a@b.c"}, "password": {"bad"}})
	if err != nil {
		t.Fatalf("rate limited attempt: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", resp.StatusCode)
	}

	// The JSON API auth surface is throttled by the same rule.
	status, _ := callJSONServer(t, server, http.MethodPost, "/api/auth/login",
		`{"email":"a@b.c","password":"bad"}`, "")
	if status != http.StatusTooManyRequests {
		t.Fatalf("api login: want 429, got %d", status)
	}
}

// newMockGoogleProvider simulates the provider endpoints the manager
// overrides: authorize (bounces straight back), token, and userinfo.
func newMockGoogleProvider(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		target := q.Get("redirect_uri") + "?code=mock-code&state=" + url.QueryEscape(q.Get("state"))
		http.Redirect(w, r, target, http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"mock-access-token","token_type":"Bearer"}`))
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"sub-google-1","email":"ada@example.com","name":"Ada","picture":""}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// TestOAuthFlowWithMockProvider drives the full google flow against a mock
// provider: session → authorize redirect → callback exchange → linked.
func TestOAuthFlowWithMockProvider(t *testing.T) {
	mock := newMockGoogleProvider(t)
	server, cfg, rebuild := newSecurityServer(t, func(cfg *config.Config) {
		cfg.GoogleClientID = "test-client"
		cfg.GoogleSecret = "test-secret"
		cfg.GoogleAuthURL = mock.URL + "/auth"
		cfg.GoogleTokenURL = mock.URL + "/token"
		cfg.GoogleUserInfo = mock.URL + "/userinfo"
	})
	// The manager bakes the redirect URI from BaseURL at construction; point
	// it at the real server and rebuild.
	cfg.BaseURL = server.URL
	rebuild()

	// A client that reports redirects instead of following them.
	noFollow := *server.Client()
	noFollow.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// 1. Register a user to obtain a session.
	status, body := callJSONServer(t, server, http.MethodPost, "/api/auth/register",
		`{"email":"ada@example.com","name":"Ada","password":"password123"}`, "")
	if status != http.StatusCreated {
		t.Fatalf("register: want 201, got %d", status)
	}
	token, _ := body["token"].(string)

	// 2. Start the flow: must redirect to the mock provider with state.
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/auth/google/login", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	start, err := noFollow.Do(req)
	if err != nil {
		t.Fatalf("oauth login: %v", err)
	}
	redirectURL, err := url.Parse(start.Header.Get("Location"))
	_ = start.Body.Close()
	if err != nil || start.StatusCode != http.StatusSeeOther {
		t.Fatalf("oauth login redirect: %d %v", start.StatusCode, err)
	}
	if !strings.HasPrefix(redirectURL.Query().Get("redirect_uri"), server.URL) {
		t.Fatalf("redirect_uri: %q", redirectURL.Query().Get("redirect_uri"))
	}
	state := redirectURL.Query().Get("state")
	if state == "" {
		t.Fatal("authorize url is missing state")
	}

	// 3. The mock provider bounces back with a code; the app exchanges it.
	callback := server.URL + "/auth/google/callback?code=mock-code&state=" + url.QueryEscape(state)
	req, _ = http.NewRequest(http.MethodGet, callback, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	cb, err := noFollow.Do(req)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	cbBody, _ := io.ReadAll(cb.Body)
	_ = cb.Body.Close()
	if cb.StatusCode != http.StatusSeeOther {
		t.Fatalf("callback: want 303, got %d (%s)", cb.StatusCode, cbBody)
	}

	// 4. The profile page now lists the linked google identity.
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	profile, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	profileBody, _ := io.ReadAll(profile.Body)
	_ = profile.Body.Close()
	if !strings.Contains(string(profileBody), "Linked providers") {
		t.Fatalf("profile page did not render (%.300s)", profileBody)
	}
	if !strings.Contains(string(profileBody), "google") || !strings.Contains(string(profileBody), "ada@example.com") {
		t.Errorf("profile: google identity missing (%.300s)", profileBody)
	}

	// 5. The state cannot be replayed.
	req, _ = http.NewRequest(http.MethodGet, callback, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	replay, err := noFollow.Do(req)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	_ = replay.Body.Close()
	if replay.StatusCode != http.StatusBadRequest {
		t.Errorf("state replay: want 400, got %d", replay.StatusCode)
	}
}

func callJSONServer(t *testing.T, server *httptest.Server, method, path, body, token string) (int, map[string]any) {
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
		t.Fatalf("%s %s: decode: %v", method, path, err)
	}
	return resp.StatusCode, parsed
}
