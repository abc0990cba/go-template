package observ

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func redactLines(t *testing.T, log func(*slog.Logger)) []string {
	t.Helper()
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(NewRedactHandler(handler))
	log(logger)
	out := buf.String()
	if out == "" {
		t.Fatal("nothing logged")
	}
	return strings.Split(strings.TrimRight(out, "\n"), "\n")
}

func TestRedactHandlerMasksSensitiveKeys(t *testing.T) {
	lines := redactLines(t, func(logger *slog.Logger) {
		logger.Info("login attempt", "email", "ada@example.com", "password", "hunter2")
		logger.Warn("oauth exchange", "Authorization", "Bearer abc.def.ghi")
		logger.Info("mfa enroll", "recovery_codes", []string{"1111-2222", "3333-4444"})
	})
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "hunter2") || strings.Contains(joined, "abc.def.ghi") {
		t.Fatalf("secret leaked into logs: %s", joined)
	}
	if !strings.Contains(lines[0], "password=[REDACTED]") {
		t.Fatalf("password key not redacted: %s", lines[0])
	}
	// The email KEY gets the masked form (correlatable, not publishable).
	if !strings.Contains(lines[0], "email=a***@example.com") {
		t.Fatalf("email key not masked: %s", lines[0])
	}
	if !strings.Contains(lines[1], "Authorization=[REDACTED]") {
		t.Fatalf("authorization not redacted: %s", lines[1])
	}
	if !strings.Contains(lines[2], "recovery_codes=[REDACTED]") {
		t.Fatalf("grouped/codes key not redacted: %s", lines[2])
	}
}

func TestRedactHandlerMasksEmailsInsideStringValues(t *testing.T) {
	lines := redactLines(t, func(logger *slog.Logger) {
		logger.Info("invitation sent", "target", "invite grace <ada.lovelace@example-corp.com> via relay")
	})
	if strings.Contains(lines[0], "ada.lovelace@example-corp.com") {
		t.Fatalf("email leaked into a value: %s", lines[0])
	}
	if !strings.Contains(lines[0], "a***@example-corp.com") {
		t.Fatalf("email not masked as expected: %s", lines[0])
	}
	// The surrounding text survives.
	if !strings.Contains(lines[0], "invite grace <") || !strings.Contains(lines[0], "> via relay") {
		t.Fatalf("surrounding text damaged: %s", lines[0])
	}
}

func TestRedactHandlerLeavesInnocentValuesAlone(t *testing.T) {
	lines := redactLines(t, func(logger *slog.Logger) {
		logger.Info("request", "method", "POST", "path", "/api/users", "status", 200, "duration_ms", 12)
	})
	if !strings.Contains(lines[0], `path=/api/users`) || !strings.Contains(lines[0], "status=200") {
		t.Fatalf("innocent attributes changed: %s", lines[0])
	}
	if !strings.Contains(lines[0], "msg=request") {
		t.Fatalf("message changed: %s", lines[0])
	}
}

func TestRedactHandlerJSONOutputStaysParseable(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(NewRedactHandler(handler))
	logger.Info("with group", slog.Group("auth", slog.String("token", "super-secret"), slog.String("user", "u1")))

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("json output broken: %v", err)
	}
	authGroup, _ := parsed["auth"].(map[string]any)
	if authGroup == nil {
		t.Fatalf("group missing: %s", buf.String())
	}
	if authGroup["token"] != "[REDACTED]" {
		t.Fatalf("grouped token leaked: %v", authGroup)
	}
	if authGroup["user"] != "u1" {
		t.Fatalf("grouped innocent value changed: %v", authGroup)
	}
}

func TestMaskEmails(t *testing.T) {
	cases := []struct{ in, want string }{
		{"no email here", "no email here"},
		{"", ""},
		{"@weird", "@weird"},
		{"ada@example.com", "a***@example.com"},
		{"mail bob<bob.smith@corp.io> ok", "mail bob<b***@corp.io> ok"},
		{"two: a@x.io and b@y.io", "two: a***@x.io and b***@y.io"},
		{"not an email: user@", "not an email: user@"},
		{"trailing@dot.example.", "t***@dot.example."},
	}
	for _, tc := range cases {
		if got := maskEmails(tc.in); got != tc.want {
			t.Fatalf("maskEmails(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
