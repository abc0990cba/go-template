package httpapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/tokens"
)

// newTokenTestServer builds a router with the JWT grant enabled. The shared
// test container loads config from the environment (no secret there), so the
// token service is rebuilt against the same stores with a test secret.
func newTokenTestServer(t *testing.T, cfg config.Config) (*httptest.Server, *repository.MemoryStore) {
	t.Helper()
	logger := discardLogger()
	store := repository.NewMemoryStore()
	services := newTestContainer(t, store, nil, logger)
	services.Tokens = tokens.NewService(store, store, services.MFA, store, cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL, auth.VerifyPassword)
	server := httptest.NewServer(NewRouter(cfg, services, logger, store, observ.Build{}, realtime.NewHub()))
	t.Cleanup(server.Close)
	return server, store
}

// loadTestConfigWithJWT returns config with the token grant turned on.
func loadTestConfigWithJWT(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.JWTSecret = "router-test-secret-0123456789abcdef"
	return cfg
}

func TestTokenGrantLifecycle(t *testing.T) {
	server, _ := newTokenTestServer(t, loadTestConfigWithJWT(t))
	email := "jwt-flow@example.com"
	registerAndLogin(t, server, email)

	// Password grant issues a working access token.
	status, body := callJSON(t, server, http.MethodPost, "/api/auth/token",
		`{"grant_type":"password","email":"`+email+`","password":"password123"}`, "")
	if status != http.StatusOK {
		t.Fatalf("grant: want 200, got %d (%v)", status, body)
	}
	access, _ := body["access_token"].(string)
	refresh, _ := body["refresh_token"].(string)
	if access == "" || refresh == "" {
		t.Fatalf("want access + refresh tokens, got %v", body)
	}
	status, _ = callJSON(t, server, http.MethodGet, "/api/me", "", access)
	if status != http.StatusOK {
		t.Fatalf("access token must authenticate /api/me: got %d", status)
	}

	// Rotation: refresh once, get a fresh pair.
	status, body = callJSON(t, server, http.MethodPost, "/api/auth/token",
		`{"grant_type":"refresh_token","refresh_token":"`+refresh+`"}`, "")
	if status != http.StatusOK {
		t.Fatalf("refresh: want 200, got %d (%v)", status, body)
	}
	secondRefresh, _ := body["refresh_token"].(string)
	if secondRefresh == "" || secondRefresh == refresh {
		t.Fatal("refresh must rotate the token")
	}

	// Reuse of the rotated token triggers family revocation.
	status, body = callJSON(t, server, http.MethodPost, "/api/auth/token",
		`{"grant_type":"refresh_token","refresh_token":"`+refresh+`"}`, "")
	if status != http.StatusUnauthorized || !strings.Contains(titleOf(t, body), "reuse") {
		t.Fatalf("reuse: want 401 reuse title, got %d (%v)", status, body)
	}
	status, _ = callJSON(t, server, http.MethodPost, "/api/auth/token",
		`{"grant_type":"refresh_token","refresh_token":"`+secondRefresh+`"}`, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("family must be dead after reuse: got %d", status)
	}

	// Bad grants stay uniform 401s.
	status, _ = callJSON(t, server, http.MethodPost, "/api/auth/token",
		`{"grant_type":"password","email":"`+email+`","password":"wrong"}`, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("bad password: want 401, got %d", status)
	}
}

func TestTokenGrantDisabledWithoutSecret(t *testing.T) {
	server := newTestServer(t) // test environment sets no APP_JWT_SECRET
	status, body := callJSON(t, server, http.MethodPost, "/api/auth/token",
		`{"grant_type":"password","email":"x@example.com","password":"whatever1"}`, "")
	if status != http.StatusNotImplemented {
		t.Fatalf("disabled token endpoint: want 501, got %d (%v)", status, body)
	}
}

func TestJWTBearerResolvesOnAuthorizedRoutes(t *testing.T) {
	server, _ := newTokenTestServer(t, loadTestConfigWithJWT(t))
	email := "jwt-bearer@example.com"
	registerAndLogin(t, server, email)

	status, body := callJSON(t, server, http.MethodPost, "/api/auth/token",
		`{"grant_type":"password","email":"`+email+`","password":"password123"}`, "")
	if status != http.StatusOK {
		t.Fatalf("grant: got %d (%v)", status, body)
	}
	access, _ := body["access_token"].(string)

	status, me := callJSON(t, server, http.MethodGet, "/api/me", "", access)
	if status != http.StatusOK {
		t.Fatalf("/api/me with JWT: got %d (%v)", status, me)
	}
	// /api/me is a flat session probe with the user fields at the top level.
	if me["email"] != email {
		t.Fatalf("JWT must resolve the right user, got %v", me["email"])
	}

	// Garbage JWTs are plain 401s.
	status, _ = callJSON(t, server, http.MethodGet, "/api/me", "", "abc.def.ghi")
	if status != http.StatusUnauthorized {
		t.Fatalf("garbage JWT: want 401, got %d", status)
	}
}

// titleOf reads the problem title out of a decoded response.
func titleOf(t *testing.T, body map[string]any) string {
	t.Helper()
	title, _ := body["title"].(string)
	return title
}
