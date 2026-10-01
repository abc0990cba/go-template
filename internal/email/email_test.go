// External test package: the memory store (repository) implements both
// email ports (Store and AuthWriter), which production wiring passes as
// two separate dependencies.
package email_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/mailer"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/users"
)

// captureMailer records every outgoing message; failNext makes Send return
// an error to prove the flow survives mail failures.
type captureMailer struct {
	sent     []mailer.Message
	failNext bool
}

func (m *captureMailer) Send(_ context.Context, msg mailer.Message) error {
	m.sent = append(m.sent, msg)
	if m.failNext {
		m.failNext = false
		return errors.New("smtp unreachable")
	}
	return nil
}

// linkOf returns the href of the action button in a sent message.
func linkOf(msg mailer.Message) string {
	body := string(msg.Body)
	start := strings.Index(body, `href="`)
	if start < 0 {
		return ""
	}
	rest := body[start+len(`href="`):]
	return rest[:strings.Index(rest, `"`)]
}

// tokenOf pulls the single-use token out of a sent message's link.
func tokenOf(t *testing.T, msg mailer.Message) string {
	t.Helper()
	parsed, err := url.Parse(linkOf(msg))
	if err != nil {
		t.Fatalf("parse link %q: %v", linkOf(msg), err)
	}
	token := parsed.Query().Get("token")
	if token == "" {
		t.Fatalf("link %q carries no token", linkOf(msg))
	}
	return token
}

type fixture struct {
	svc      *email.Service
	accounts *auth.Service
	store    *repository.MemoryStore
	mailer   *captureMailer
	notify   []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	store := repository.NewMemoryStore()
	sender := &captureMailer{}
	svc := email.NewService(store, store, sender, auth.HashPassword, "https://app.example.com/", discardLogger())
	accounts := auth.NewService(store, nil, time.Hour)
	f := &fixture{svc: svc, accounts: accounts, store: store, mailer: sender}
	svc.WithNotifier(func(_ context.Context, _, typ, _, _, _ string) {
		f.notify = append(f.notify, typ)
	})
	ctx := context.Background()
	if _, _, err := accounts.Register(ctx, "ada@example.com", "Ada", "password123"); err != nil {
		t.Fatalf("register: %v", err)
	}
	return f
}

func (f *fixture) ada(t *testing.T) users.User {
	t.Helper()
	user, err := f.accounts.UserByEmail(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatalf("load ada: %v", err)
	}
	return user
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRequestPasswordResetFlow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// Unknown addresses are answered like known ones: no email, no error,
	// no account-existence leak.
	if err := f.svc.RequestPasswordReset(ctx, "ghost@example.com"); err != nil {
		t.Fatalf("unknown email reset: want nil, got %v", err)
	}
	if len(f.mailer.sent) != 0 {
		t.Errorf("unknown email: want no mail, got %d", len(f.mailer.sent))
	}

	// The lookup is case-insensitive like every email path.
	if err := f.svc.RequestPasswordReset(ctx, "ADA@Example.COM"); err != nil {
		t.Fatalf("reset request: %v", err)
	}
	if len(f.mailer.sent) != 1 {
		t.Fatalf("reset request: want 1 email, got %d", len(f.mailer.sent))
	}
	msg := f.mailer.sent[0]
	if msg.To != "ada@example.com" {
		t.Errorf("reset email to: want ada@example.com, got %q", msg.To)
	}
	if !strings.Contains(msg.Subject, "Reset") {
		t.Errorf("reset subject: got %q", msg.Subject)
	}
	tokenOf(t, msg)

	// Mail failure must not fail the flow or surface to the caller.
	f.mailer.failNext = true
	if err := f.svc.RequestPasswordReset(ctx, "ada@example.com"); err != nil {
		t.Errorf("reset with failing mailer: want nil, got %v", err)
	}
}

func TestResetPasswordConsumesTokenOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.svc.RequestPasswordReset(ctx, "ada@example.com"); err != nil {
		t.Fatalf("reset request: %v", err)
	}
	token := tokenOf(t, f.mailer.sent[0])

	// A short password is a validation error and must not burn the token.
	if err := f.svc.ResetPassword(ctx, token, "short"); !kernel.IsValidationError(err) {
		t.Fatalf("short password: want validation error, got %v", err)
	}

	// A wrong token is a validation error, never a 500.
	if err := f.svc.ResetPassword(ctx, "not-a-token", "newpassword123"); !kernel.IsValidationError(err) {
		t.Fatalf("wrong token: want validation error, got %v", err)
	}

	// The reset accepts the new password and rejects the old one.
	if err := f.svc.ResetPassword(ctx, token, "newpassword123"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, _, err := f.accounts.Login(ctx, "ada@example.com", "newpassword123"); err != nil {
		t.Errorf("login with new password: %v", err)
	}
	if _, _, err := f.accounts.Login(ctx, "ada@example.com", "password123"); !errors.Is(err, kernel.ErrInvalidCredentials) {
		t.Errorf("old password after reset: want ErrInvalidCredentials, got %v", err)
	}

	// Single use: the consumed token is dead.
	if err := f.svc.ResetPassword(ctx, token, "anotherpass123"); !kernel.IsValidationError(err) {
		t.Errorf("token reuse: want validation error, got %v", err)
	}

	// The notifier fired exactly once for the password change.
	if len(f.notify) != 1 || f.notify[0] != "security.password_changed" {
		t.Errorf("notifier: want [security.password_changed], got %v", f.notify)
	}
}

func TestVerifyEmailFlow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if err := f.svc.SendVerification(ctx, f.ada(t)); err != nil {
		t.Fatalf("send verification: %v", err)
	}
	if len(f.mailer.sent) != 1 {
		t.Fatalf("verification: want 1 email, got %d", len(f.mailer.sent))
	}
	token := tokenOf(t, f.mailer.sent[0])

	// The happy path consumes the token.
	if err := f.svc.VerifyEmail(ctx, token); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := f.svc.VerifyEmail(ctx, token); !kernel.IsValidationError(err) {
		t.Errorf("verify reuse: want validation error, got %v", err)
	}

	// Kinds never mix: a fresh reset token cannot verify an email.
	if err := f.svc.RequestPasswordReset(ctx, "ada@example.com"); err != nil {
		t.Fatalf("reset request: %v", err)
	}
	resetToken := tokenOf(t, f.mailer.sent[1])
	if err := f.svc.VerifyEmail(ctx, resetToken); !kernel.IsValidationError(err) {
		t.Errorf("reset token as verify token: want validation error, got %v", err)
	}

	// Empty and malformed tokens are validation errors, not 500s.
	if err := f.svc.VerifyEmail(ctx, ""); !kernel.IsValidationError(err) {
		t.Errorf("empty token: want validation error, got %v", err)
	}
}
