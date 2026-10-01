// The email HTTP surface: uniform answers that never leak which addresses
// exist, the feature-flag gate, and validation failures as 400.
package email_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/settings"
)

// newEmailServer mounts the email routes the way the assembly does, with a
// pass-through rate limiter.
func newEmailServer(t *testing.T) (*httptest.Server, *fixture, *settings.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	f := newFixture(t)
	store := f.store
	cfg := settings.NewService(store)
	router := gin.New()
	email.RegisterHTTP(router.Group("/api"), email.HTTP{
		Svc:           f.svc,
		Settings:      cfg,
		Audit:         audit.NewService(store),
		AuthRateLimit: func(c *gin.Context) { c.Next() },
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server, f, cfg
}

// post sends one JSON request and answers with the status code.
func post(t *testing.T, server *httptest.Server, path, body string) int {
	t.Helper()
	resp, err := server.Client().Post(server.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func TestForgotPasswordHTTP(t *testing.T) {
	server, f, cfg := newEmailServer(t)
	ctx := context.Background()

	// The endpoint answers 204 regardless: success, unknown address, and a
	// malformed body are indistinguishable from the outside — except that
	// only the known address sends mail.
	if status := post(t, server, "/api/auth/forgot-password", `{"email":"ada@example.com"}`); status != http.StatusNoContent {
		t.Errorf("forgot-password: want 204, got %d", status)
	}
	if len(f.mailer.sent) != 1 {
		t.Errorf("forgot-password: want 1 email, got %d", len(f.mailer.sent))
	}
	if status := post(t, server, "/api/auth/forgot-password", `{"email":"ghost@example.com"}`); status != http.StatusNoContent {
		t.Errorf("forgot-password unknown: want 204, got %d", status)
	}
	if status := post(t, server, "/api/auth/forgot-password", `{"email":""}`); status != http.StatusBadRequest {
		t.Errorf("forgot-password empty email: want 400, got %d", status)
	}
	if status := post(t, server, "/api/auth/forgot-password", `not json`); status != http.StatusBadRequest {
		t.Errorf("forgot-password bad body: want 400, got %d", status)
	}

	// The feature flag short-circuits the flow before any mail is built.
	if err := cfg.Set(ctx, "password_reset.enabled", "false", "tester"); err != nil {
		t.Fatalf("disable password_reset: %v", err)
	}
	before := len(f.mailer.sent)
	if status := post(t, server, "/api/auth/forgot-password", `{"email":"ada@example.com"}`); status != http.StatusNoContent {
		t.Errorf("forgot-password disabled: want 204, got %d", status)
	}
	if len(f.mailer.sent) != before {
		t.Error("forgot-password disabled: want no mail")
	}
}

func TestResetAndVerifyPasswordHTTP(t *testing.T) {
	server, f, _ := newEmailServer(t)

	if status := post(t, server, "/api/auth/reset-password", `{"token":"bogus","password":"newpassword123"}`); status != http.StatusBadRequest {
		t.Errorf("reset with bogus token: want 400, got %d", status)
	}
	if status := post(t, server, "/api/auth/reset-password", `{"token":"x","password":"short"}`); status != http.StatusBadRequest {
		t.Errorf("reset with short password: want 400, got %d", status)
	}

	if status := post(t, server, "/api/auth/forgot-password", `{"email":"ada@example.com"}`); status != http.StatusNoContent {
		t.Fatalf("forgot-password: want 204, got %d", status)
	}
	token := tokenOf(t, f.mailer.sent[0])
	// The same token works exactly once over HTTP.
	if status := post(t, server, "/api/auth/reset-password", `{"token":"`+token+`","password":"newpassword123"}`); status != http.StatusNoContent {
		t.Errorf("reset: want 204, got %d", status)
	}
	if status := post(t, server, "/api/auth/reset-password", `{"token":"`+token+`","password":"anotherpass123"}`); status != http.StatusBadRequest {
		t.Errorf("reset reuse: want 400, got %d", status)
	}

	// Verification over HTTP: one 204, then the token is spent.
	if err := f.svc.SendVerification(context.Background(), f.ada(t)); err != nil {
		t.Fatalf("send verification: %v", err)
	}
	verifyToken := tokenOf(t, f.mailer.sent[1])
	if status := post(t, server, "/api/auth/verify-email", `{"token":"`+verifyToken+`"}`); status != http.StatusNoContent {
		t.Errorf("verify: want 204, got %d", status)
	}
	if status := post(t, server, "/api/auth/verify-email", `{"token":"`+verifyToken+`"}`); status != http.StatusBadRequest {
		t.Errorf("verify reuse: want 400, got %d", status)
	}
}
