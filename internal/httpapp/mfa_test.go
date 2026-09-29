package httpapp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestMFALoginRequiresSecondStep(t *testing.T) {
	server := newTestServer(t)
	token := registerAndLogin(t, server, "mfa-flow@example.com")

	// Enroll through the API: start, then confirm with a real TOTP code.
	status, body := callJSON(t, server, http.MethodPost, "/api/auth/mfa/enroll", "", token)
	if status != http.StatusOK {
		t.Fatalf("enroll: want 200, got %d (%v)", status, body)
	}
	secret, _ := body["secret"].(string)
	if secret == "" {
		t.Fatal("enroll must return a secret")
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	status, body = callJSON(t, server, http.MethodPost, "/api/auth/mfa/verify",
		fmt.Sprintf(`{"code":%q}`, code), token)
	if status != http.StatusOK {
		t.Fatalf("verify: want 200, got %d (%v)", status, body)
	}
	codes, _ := body["recovery_codes"].([]any)
	if len(codes) == 0 {
		t.Fatal("verify must return recovery codes")
	}

	// Login now answers with the challenge, not a session.
	status, body = callJSON(t, server, http.MethodPost, "/api/auth/login",
		`{"email":"mfa-flow@example.com","password":"password123"}`, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("login: want 401 challenge, got %d (%v)", status, body)
	}
	if required, _ := body["mfa_required"].(bool); !required {
		t.Fatalf("want mfa_required flag, got %v", body)
	}
	challenge, _ := body["challenge"].(string)

	// A bogus code fails; the session stays locked.
	status, _ = callJSON(t, server, http.MethodPost, "/api/auth/mfa/challenge",
		fmt.Sprintf(`{"challenge":%q,"code":"000000"}`, challenge), "")
	if status == http.StatusOK {
		t.Fatal("bogus code must not open a session")
	}

	// The right code completes the login.
	code, err = totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	status, body = callJSON(t, server, http.MethodPost, "/api/auth/mfa/challenge",
		fmt.Sprintf(`{"challenge":%q,"code":%q}`, challenge, code), "")
	if status != http.StatusOK {
		t.Fatalf("challenge: want 200, got %d (%v)", status, body)
	}
	session, _ := body["token"].(string)
	if session == "" {
		t.Fatal("completed challenge must return a session token")
	}
	status, _ = callJSON(t, server, http.MethodGet, "/api/me", "", session)
	if status != http.StatusOK {
		t.Fatalf("session from challenge must authenticate: got %d", status)
	}

	// Status reports enabled; disable requires password plus a code.
	status, body = callJSON(t, server, http.MethodGet, "/api/auth/mfa/status", "", session)
	if status != http.StatusOK {
		t.Fatalf("status: got %d", status)
	}
	if enabled, _ := body["enabled"].(bool); !enabled {
		t.Fatalf("status must report enabled, got %v", body)
	}
	code, err = totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	status, _ = callJSON(t, server, http.MethodPost, "/api/auth/mfa/disable",
		fmt.Sprintf(`{"password":"password123","code":%q}`, code), session)
	if status != http.StatusNoContent {
		t.Fatalf("disable: want 204, got %d", status)
	}
	status, _ = callJSON(t, server, http.MethodPost, "/api/auth/login",
		`{"email":"mfa-flow@example.com","password":"password123"}`, "")
	if status != http.StatusOK {
		t.Fatalf("login after disable: want 200, got %d", status)
	}
}

func TestMFAResetRequiresPermission(t *testing.T) {
	server := newTestServer(t)
	admin := registerAndLogin(t, server, "mfa-admin@example.com") // first account = superadmin
	viewer := registerAndLogin(t, server, "mfa-viewer@example.com")
	target := registerAndLogin(t, server, "mfa-target@example.com")

	status, _ := callJSON(t, server, http.MethodDelete, "/api/users/"+userIDOf(t, server, viewer)+"/mfa", "", viewer)
	if status != http.StatusForbidden {
		t.Fatalf("viewer reset: want 403, got %d", status)
	}
	status, _ = callJSON(t, server, http.MethodDelete, "/api/users/"+userIDOf(t, server, target)+"/mfa", "", admin)
	if status != http.StatusNoContent {
		t.Fatalf("admin reset: want 204, got %d", status)
	}
}

// userIDOf resolves a user id through /api/me.
func userIDOf(t *testing.T, server *httptest.Server, token string) string {
	t.Helper()
	_, body := callJSON(t, server, http.MethodGet, "/api/me", "", token)
	id := ""
	if user, ok := body["user"].(map[string]any); ok {
		id, _ = user["id"].(string)
	}
	if id == "" {
		id, _ = body["id"].(string)
	}
	if id == "" {
		t.Fatalf("cannot resolve user id from %v", body)
	}
	return id
}
