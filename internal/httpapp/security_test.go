package httpapp

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/platform/oauth"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/repository"
)

// newSecurityServer builds the full router.
func newSecurityServer(t *testing.T, mutate func(*config.Config)) *httptest.Server {
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
	hub := realtime.NewHub()
	// Mirror production wiring: events flow through the fan sink into the hub.
	services := newTestContainer(t, store, realtime.NewFanSink(hub), logger)
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}, hub))
	jar, _ := cookiejar.New(nil)
	server.Client().Jar = jar
	t.Cleanup(server.Close)
	return server
}

// newOAuthSecurityServer builds the full router with an OAuth manager
// constructed after the server's URL is known, so the redirect URIs baked
// into the manager point at the test server without a handler rebuild.
func newOAuthSecurityServer(t *testing.T, mutate func(*config.Config)) *httptest.Server {
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
	hub := realtime.NewHub()
	services := newTestContainer(t, store, realtime.NewFanSink(hub), logger)
	// The manager bakes the redirect URI from BaseURL at construction; the
	// placeholder listener exists only to learn the URL, and the real
	// handler is swapped in before any request is made.
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(server.Close)
	cfg.BaseURL = server.URL
	server.Config.Handler = NewRouter(cfg, services, logger, store, observ.Build{}, hub, Options{OAuth: oauth.NewManager(cfg)})
	jar, _ := cookiejar.New(nil)
	server.Client().Jar = jar
	return server
}

// TestOriginCheckRejectsCrossSitePosts replaces the old HTML-form CSRF
// double-submit check: browsers are now protected by Origin/Sec-Fetch-Site
// validation on unsafe API methods plus the SameSite=Lax session cookie.
func TestOriginCheckRejectsCrossSitePosts(t *testing.T) {
	server := newSecurityServer(t, nil)
	token := registerAndLogin(t, server, "ada@example.com")

	emailN := 0
	post := func(headers map[string]string) int {
		t.Helper()
		emailN++
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/users",
			strings.NewReader(`{"email":"new-`+strconv.Itoa(emailN)+`@example.com","name":"New"}`))
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}

	if status := post(map[string]string{"Origin": "https://evil.example"}); status != http.StatusForbidden {
		t.Errorf("cross-origin post: want 403, got %d", status)
	}
	if status := post(map[string]string{"Sec-Fetch-Site": "cross-site"}); status != http.StatusForbidden {
		t.Errorf("cross-site fetch post: want 403, got %d", status)
	}
	// The SPA's own origin passes and the create succeeds.
	if status := post(map[string]string{"Origin": server.URL, "Sec-Fetch-Site": "same-origin"}); status != http.StatusCreated {
		t.Errorf("same-origin post: want 201, got %d", status)
	}
	// GETs are never blocked; non-browser clients send no Origin at all.
	if status := post(nil); status != http.StatusCreated {
		t.Errorf("post without browser headers: want 201, got %d", status)
	}
}

func TestSecurityHeadersPresent(t *testing.T) {
	server := newSecurityServer(t, nil)
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
	server := newSecurityServer(t, func(cfg *config.Config) {
		cfg.AuthRateBurst = 3
		cfg.AuthRateRPS = 0.001
	})

	// The first burst passes; the next request from the same IP is denied.
	for i := 0; i < 3; i++ {
		status, _ := callJSONServer(t, server, "/api/auth/login", `{"email":"a@b.c","password":"bad"}`)
		if status == http.StatusTooManyRequests {
			t.Fatalf("attempt %d hit the limit too early", i)
		}
	}
	status, _ := callJSONServer(t, server, "/api/auth/login", `{"email":"a@b.c","password":"bad"}`)
	if status != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", status)
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
	server := newOAuthSecurityServer(t, func(cfg *config.Config) {
		cfg.GoogleClientID = "test-client"
		cfg.GoogleSecret = "test-secret"
		cfg.GoogleAuthURL = mock.URL + "/auth"
		cfg.GoogleTokenURL = mock.URL + "/token"
		cfg.GoogleUserInfo = mock.URL + "/userinfo"
	})

	// A client that reports redirects instead of following them.
	noFollow := *server.Client()
	noFollow.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// 1. Register a user to obtain a session.
	status, body := callJSONServer(t, server, "/api/auth/register", `{"email":"ada@example.com","name":"Ada","password":"password123"}`)
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

	// 4. The identities endpoint now lists the linked google identity.
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/api/me/identities", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	profile, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("identities: %v", err)
	}
	profileBody, _ := io.ReadAll(profile.Body)
	_ = profile.Body.Close()
	var parsed struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(profileBody, &parsed); err != nil {
		t.Fatalf("identities decode: %v (%s)", err, profileBody)
	}
	found := false
	for _, identity := range parsed.Data {
		if identity["provider"] == "google" && identity["email"] == "ada@example.com" {
			found = true
		}
	}
	if !found {
		t.Errorf("google identity missing after link: %s", profileBody)
	}

	// 5. The state cannot be replayed.
	req, _ = http.NewRequest(http.MethodGet, callback, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	replay, err := noFollow.Do(req)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	_ = replay.Body.Close()
	if replay.StatusCode != http.StatusSeeOther {
		t.Errorf("state replay: want 303 to profile error, got %d", replay.StatusCode)
	}
}

// newMockTwitterProvider simulates Twitter's OAuth2 endpoints and enforces
// the PKCE contract: the authorize request must carry an S256 challenge, and
// the token request must present the verifier that hashes back to it.
func newMockTwitterProvider(t *testing.T) *httptest.Server {
	t.Helper()
	var challenge string
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
			http.Error(w, "missing PKCE challenge", http.StatusBadRequest)
			return
		}
		challenge = q.Get("code_challenge")
		target := q.Get("redirect_uri") + "?code=mock-code&state=" + url.QueryEscape(q.Get("state"))
		http.Redirect(w, r, target, http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad token form", http.StatusBadRequest)
			return
		}
		verifier := r.PostFormValue("code_verifier")
		sum := sha256.Sum256([]byte(verifier))
		if verifier == "" || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
			http.Error(w, "PKCE verification failed", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"mock-access-token","token_type":"Bearer"}`))
	})
	mux.HandleFunc("/users/me", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"987654","name":"Ada","username":"ada","profile_image_url":"https://pbs.twimg.com/profile_images/ada_normal.jpg"}}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// TestOAuthFlowWithPKCEProvider drives the twitter flow end to end; Twitter
// mandates PKCE, so a broken challenge/verifier round trip fails the mock
// token endpoint and the link never happens.
func TestOAuthFlowWithPKCEProvider(t *testing.T) {
	mock := newMockTwitterProvider(t)
	server := newOAuthSecurityServer(t, func(cfg *config.Config) {
		cfg.TwitterClientID = "test-client"
		cfg.TwitterSecret = "test-secret"
		cfg.TwitterAuthURL = mock.URL + "/auth"
		cfg.TwitterTokenURL = mock.URL + "/token"
		cfg.TwitterUserInfo = mock.URL + "/users/me"
	})

	noFollow := *server.Client()
	noFollow.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	status, body := callJSONServer(t, server, "/api/auth/register", `{"email":"ada@example.com","name":"Ada","password":"password123"}`)
	if status != http.StatusCreated {
		t.Fatalf("register: want 201, got %d", status)
	}
	token, _ := body["token"].(string)

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/auth/twitter/login", nil)
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
	if redirectURL.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize url is missing the PKCE challenge: %s", redirectURL)
	}
	// Visit the provider authorize endpoint so the mock validates and
	// captures the challenge, as a real browser would.
	authorize, err := noFollow.Get(redirectURL.String())
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	_ = authorize.Body.Close()
	if authorize.StatusCode != http.StatusFound {
		t.Fatalf("authorize: want 302 back with a code, got %d", authorize.StatusCode)
	}
	state := redirectURL.Query().Get("state")

	callback := server.URL + "/auth/twitter/callback?code=mock-code&state=" + url.QueryEscape(state)
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
	if strings.Contains(cb.Header.Get("Location"), "oauth_error") {
		t.Fatalf("callback bounced with an oauth error: %s", cb.Header.Get("Location"))
	}

	// The linked identity carries the twitter profile (no email — Twitter
	// does not expose one; the subject stands in).
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/api/me/identities", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	identities, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("identities: %v", err)
	}
	identityBody, _ := io.ReadAll(identities.Body)
	_ = identities.Body.Close()
	var parsed struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(identityBody, &parsed); err != nil {
		t.Fatalf("identities decode: %v (%s)", err, identityBody)
	}
	found := false
	for _, identity := range parsed.Data {
		if identity["provider"] == "twitter" && identity["provider_subject"] == "987654" && identity["name"] == "Ada" {
			found = true
		}
	}
	if !found {
		t.Errorf("twitter identity missing after link: %s", identityBody)
	}
}

// TestSSEStreamDeliversAnalyticsEvents reads the live stream while a login
// generates an event through the fan sink.
func TestSSEStreamDeliversAnalyticsEvents(t *testing.T) {
	server := newSecurityServer(t, nil)
	registerAndLogin(t, server, "stream@example.com")

	stream := openSSE(t, server)
	defer func() { _ = stream.resp.Body.Close() }()

	first := readSSEData(t, stream, 5*time.Second)
	if !strings.Contains(first, "connected") {
		t.Fatalf("first stream event should be the handshake, got %.200s", first)
	}

	// A login emits user_logged_in through the fan sink into the hub.
	go func() {
		_, _ = callJSONServer(t, server, "/api/auth/login", `{"email":"stream@example.com","password":"password123"}`)
	}()

	for {
		data := readSSEData(t, stream, 5*time.Second)
		if strings.Contains(data, "user_logged_in") {
			return
		}
		// Snapshots and unrelated events keep the loop reading.
	}
}

// openSSE subscribes to the event stream with the session cookie.
func openSSE(t *testing.T, server *httptest.Server) struct {
	resp   *http.Response
	reader *bufio.Reader
} {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/events/stream", nil)
	if err != nil {
		t.Fatalf("build stream request: %v", err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("open stream: want 200, got %d", resp.StatusCode)
	}
	return struct {
		resp   *http.Response
		reader *bufio.Reader
	}{resp: resp, reader: bufio.NewReader(resp.Body)}
}

// readSSEData returns the payload of the next data line, failing on timeout.
func readSSEData(t *testing.T, stream struct {
	resp   *http.Response
	reader *bufio.Reader
}, timeout time.Duration) string {
	t.Helper()
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		for {
			line, err := stream.reader.ReadString('\n')
			if err != nil {
				ch <- result{"", err}
				return
			}
			if strings.HasPrefix(line, "data: ") {
				ch <- result{strings.TrimSpace(strings.TrimPrefix(line, "data: ")), nil}
				return
			}
		}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("read stream: %v", r.err)
		}
		return r.line
	case <-time.After(timeout):
		t.Fatalf("no stream data within %s", timeout)
		return ""
	}
}

func callJSONServer(t *testing.T, server *httptest.Server, path, body string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(http.MethodPost, server.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("request %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var parsed map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil && err.Error() != "EOF" {
		t.Fatalf("%s: decode: %v", path, err)
	}
	return resp.StatusCode, parsed
}
