package httpapp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/your-org/go-template/internal/bootstrap"
	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/repository"
)

// chunkedOversizeLogin posts valid JSON whose length is hidden from
// net/http, which switches the request to chunked framing; the decoder hits
// the cap mid-parse and a.bind maps that to 413.
func chunkedOversizeLogin(t *testing.T, server *httptest.Server) int {
	t.Helper()
	prefix := `{"email":"` + strings.Repeat("x", maxBodyBytes)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/auth/login",
		io.MultiReader(strings.NewReader(prefix), strings.NewReader(`"}`)))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("chunked login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func TestChunkedOversizeBodyIs413(t *testing.T) {
	server := newTestServer(t)
	if got := chunkedOversizeLogin(t, server); got != http.StatusRequestEntityTooLarge {
		t.Fatalf("chunked oversize body: want 413, got %d", got)
	}
}

func TestPredeclaredOversizeBodyIs413(t *testing.T) {
	server := newTestServer(t)
	// Malformed junk above the cap is rejected before handlers run.
	body := bytes.Repeat([]byte("x"), maxBodyBytes+1024)
	resp, err := server.Client().Post(server.URL+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("oversize login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("pre-declared oversize body: want 413, got %d", resp.StatusCode)
	}
}

func TestHSTSTogglesWithSecureCookies(t *testing.T) {
	t.Setenv("APP_COOKIE_SECURE", "1")
	secure := newTestServer(t)
	resp, err := secure.Client().Get(secure.URL + "/health/live")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	_ = resp.Body.Close()
	if got := resp.Header.Get("Strict-Transport-Security"); got == "" {
		t.Fatal("secure mode: want Strict-Transport-Security header, got none")
	}

	t.Setenv("APP_COOKIE_SECURE", "0")
	insecure := newTestServer(t)
	resp, err = insecure.Client().Get(insecure.URL + "/health/live")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	_ = resp.Body.Close()
	if got := resp.Header.Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("insecure mode: want no HSTS header, got %q", got)
	}
}

func newMaintenanceServer(t *testing.T) (*httptest.Server, *bootstrap.Container) {
	t.Helper()
	prevTTL := maintenanceMemoTTL
	maintenanceMemoTTL = 0 // every request re-reads the flag
	t.Cleanup(func() { maintenanceMemoTTL = prevTTL })

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	logger := discardLogger()
	store := repository.NewMemoryStore()
	services := newTestContainer(t, store, nil, logger)
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}, realtime.NewHub()))
	t.Cleanup(server.Close)
	return server, services
}

func TestMaintenanceGateBlocksAPIButKeepsControls(t *testing.T) {
	server, services := newMaintenanceServer(t)
	// The flag is off by default: the API answers.
	status, _ := callJSON(t, server, http.MethodGet, "/api/settings/public", "", "")
	if status != http.StatusOK {
		t.Fatalf("flag off: want 200, got %d", status)
	}

	// Flip the flag through the settings service (as the admin API would).
	ctx := context.Background()
	if err := services.Settings.Set(ctx, maintenanceSetting, "true", "test"); err != nil {
		t.Fatalf("enable maintenance: %v", err)
	}

	status, body := callJSON(t, server, http.MethodGet, "/api/users", "", "")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("flag on: want 503 for /api/users, got %d (%v)", status, body)
	}
	if body["title"] != "service under maintenance" {
		t.Fatalf("want problem+json title, got %v", body["title"])
	}

	resp, err := server.Client().Get(server.URL + "/api/settings/public")
	if err != nil {
		t.Fatalf("public settings during maintenance: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("public settings during maintenance: want 200, got %d", resp.StatusCode)
	}

	// Login stays reachable so operators can sign in.
	status, _ = callJSON(t, server, http.MethodPost, "/api/auth/login", `{"email":"nobody@example.com","password":"wrong"}`, "")
	if status == http.StatusServiceUnavailable {
		t.Fatal("login must answer during maintenance")
	}

	resp, err = server.Client().Get(server.URL + "/health/live")
	if err != nil {
		t.Fatalf("health during maintenance: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health during maintenance: want 200, got %d", resp.StatusCode)
	}

	if err := services.Settings.Set(ctx, maintenanceSetting, "false", "test"); err != nil {
		t.Fatalf("disable maintenance: %v", err)
	}
	status, _ = callJSON(t, server, http.MethodGet, "/api/settings/public", "", "")
	if status != http.StatusOK {
		t.Fatalf("flag off again: want 200, got %d", status)
	}
}

func TestIdentityRateLimitIsPerUser(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.UserRateRPS, cfg.UserRateBurst = 2, 2
	cfg.APIRateRPS, cfg.APIRateBurst = 1000, 1000
	cfg.AuthRateRPS, cfg.AuthRateBurst = 1000, 1000
	logger := discardLogger()
	store := repository.NewMemoryStore()
	services := newTestContainer(t, store, nil, logger)
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}, realtime.NewHub()))
	t.Cleanup(server.Close)

	token := registerAndLogin(t, server, "ratelimit@example.com")
	statuses := make([]int, 0, 5)
	for range 5 {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("/api/me: %v", err)
		}
		_ = resp.Body.Close()
		statuses = append(statuses, resp.StatusCode)
	}
	if statuses[4] != http.StatusTooManyRequests {
		t.Fatalf("per-user budget: want 429 on the 5th call, got %v", statuses)
	}
	retry := statuses[4]
	_ = retry
}

func TestSecretValueFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db_url")
	if err := os.WriteFile(path, []byte("postgres://app:secret@db/app?sslmode=disable\n"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	t.Setenv("APP_DATABASE_URL_FILE", path)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load with file secret: %v", err)
	}
	if cfg.DatabaseURL != "postgres://app:secret@db/app?sslmode=disable" {
		t.Fatalf("want trimmed file contents, got %q", cfg.DatabaseURL)
	}

	// An unreadable file fails startup with the offending variable named.
	t.Setenv("APP_SMTP_PASSWORD_FILE", filepath.Join(dir, "missing"))
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "APP_SMTP_PASSWORD_FILE") {
		t.Fatalf("want unreadable file error, got %v", err)
	}
	t.Setenv("APP_SMTP_PASSWORD_FILE", "")

	// Both forms at once is a configuration error.
	t.Setenv("APP_DATABASE_URL", "postgres://direct")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("want mutual exclusion error, got %v", err)
	}
}

func TestTracingCreatesServerSpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	})

	server := newTestServer(t)
	resp, err := server.Client().Get(server.URL + "/health/live")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	_ = resp.Body.Close()

	spans := recorder.Ended()
	if len(spans) == 0 {
		t.Fatal("want at least one ended span for the request")
	}
	var found bool
	for _, span := range spans {
		if span.Name() == "/health/live" {
			found = true
			if span.SpanKind() != trace.SpanKindServer {
				t.Fatalf("span kind: want server, got %v", span.SpanKind())
			}
		}
	}
	if !found {
		t.Fatalf("want a span named /health/live, got %d spans", len(spans))
	}
}

func TestTracingIsNoopWithoutProvider(t *testing.T) {
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(noop.NewTracerProvider())
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	server := newTestServer(t)
	resp, err := server.Client().Get(server.URL + "/health/live")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("noop tracer must not affect serving, got %d", resp.StatusCode)
	}
}

// retryAfterOn429 guards the contract that rejections carry Retry-After.
func TestRateLimitRejectionsCarryRetryAfter(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.UserRateRPS, cfg.UserRateBurst = 1, 1
	cfg.APIRateRPS, cfg.APIRateBurst = 1000, 1000
	cfg.AuthRateRPS, cfg.AuthRateBurst = 1000, 1000
	logger := discardLogger()
	store := repository.NewMemoryStore()
	services := newTestContainer(t, store, nil, logger)
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}, realtime.NewHub()))
	t.Cleanup(server.Close)

	token := registerAndLogin(t, server, "retryafter@example.com")
	var retryAfter string
	for range 4 {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("/api/me: %v", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter = resp.Header.Get("Retry-After")
		}
		_ = resp.Body.Close()
	}
	if retryAfter == "" {
		t.Fatal("429 responses must carry Retry-After")
	}
	if _, err := strconv.Atoi(retryAfter); err != nil {
		t.Fatalf("Retry-After must be an integer seconds value, got %q", retryAfter)
	}
	_ = time.Second // keep the time import tied to the memo TTL type
}
