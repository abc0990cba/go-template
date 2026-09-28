package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/platform/mailer"
)

// email token kinds.
const (
	EmailTokenReset  = "reset"
	EmailTokenVerify = "verify"
)

// EmailService implements the password reset and email verification flows
// end to end: single-use hashed tokens, mail delivery, and the account
// changes they authorize. With no mailer configured the flow still works —
// the token lands in the log instead of an inbox (dev mode).
type EmailService struct {
	tokens      EmailTokensRepo
	auth        AuthRepo
	mailer      mailer.Sender
	baseURL     string
	resetTTL    time.Duration
	verifyTTL   time.Duration
	logger      *slog.Logger
	notifyOwner func(context.Context, string, string, string, string, string)
}

// NewEmailService wires the flow. notifier is optional and receives
// in-app copies of password changes.
func NewEmailService(tokens EmailTokensRepo, auth AuthRepo, sender mailer.Sender, baseURL string, logger *slog.Logger) *EmailService {
	return &EmailService{
		tokens:    tokens,
		auth:      auth,
		mailer:    sender,
		baseURL:   strings.TrimRight(baseURL, "/"),
		resetTTL:  time.Hour,
		verifyTTL: 24 * time.Hour,
		logger:    logger,
	}
}

// WithNotifier registers the in-app notification callback (wired to the
// notifications service).
func (s *EmailService) WithNotifier(notify func(context.Context, string, string, string, string, string)) {
	s.notifyOwner = notify
}

// RequestPasswordReset emails a reset link. The answer is always nil —
// unknown emails and mail failures must not leak account existence.
func (s *EmailService) RequestPasswordReset(ctx context.Context, email string) error {
	user, _, err := s.auth.GetUserByEmail(ctx, strings.ToLower(email))
	if err != nil {
		return nil
	}
	raw, token := s.newToken(EmailTokenReset, user.ID, s.resetTTL)
	if err := s.tokens.CreateEmailToken(ctx, token, hashEmailToken(raw)); err != nil {
		return err
	}
	link := fmt.Sprintf("%s/reset-password?token=%s", s.baseURL, raw)
	body := mailer.Button(link, "Reset password")
	err = s.mailer.Send(ctx, mailer.Message{
		To:      user.Email,
		Subject: "Reset your password",
		Body:    template.HTML("<p>We received a request to reset your password.</p>" + string(body)),
	})
	if err != nil {
		// Mail problems never surface to the caller; the log has the detail
		// and the raw link at debug level keeps the flow usable without a
		// mail server (enable with APP_LOG_LEVEL=debug).
		s.log("reset email failed", err)
		s.logDebug("password reset link", "url", link)
	}
	return nil
}

// ResetPassword consumes the token and sets the new password, revoking
// every session of the account.
func (s *EmailService) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	if len(newPassword) < 8 {
		return validationError("password must be at least 8 characters")
	}
	token, err := s.consumeToken(ctx, rawToken, EmailTokenReset)
	if err != nil {
		return err
	}
	if err := s.auth.SetPasswordCredential(ctx, token.UserID, HashPassword(newPassword)); err != nil {
		return err
	}
	if err := s.auth.DeleteSessionsForUser(ctx, token.UserID); err != nil {
		return err
	}
	if s.notifyOwner != nil {
		s.notifyOwner(ctx, token.UserID, "security.password_changed", "Password changed", "Your password was reset via email.", "/profile")
	}
	return nil
}

// SendVerification emails the address-confirmation link.
func (s *EmailService) SendVerification(ctx context.Context, user model.User) error {
	raw, token := s.newToken(EmailTokenVerify, user.ID, s.verifyTTL)
	if err := s.tokens.CreateEmailToken(ctx, token, hashEmailToken(raw)); err != nil {
		return err
	}
	link := fmt.Sprintf("%s/verify-email?token=%s", s.baseURL, raw)
	body := mailer.Button(link, "Confirm email")
	return s.mailer.Send(ctx, mailer.Message{
		To:      user.Email,
		Subject: "Confirm your email",
		Body:    template.HTML("<p>Please confirm your email address.</p>" + string(body)),
	})
}

// VerifyEmail consumes a verification token. The template records who
// verified; the account model has no separate flag, so the consumed token
// is the proof.
func (s *EmailService) VerifyEmail(ctx context.Context, rawToken string) error {
	_, err := s.consumeToken(ctx, rawToken, EmailTokenVerify)
	return err
}

// consumeToken resolves and consumes a single-use token of the kind.
func (s *EmailService) consumeToken(ctx context.Context, rawToken, kind string) (model.EmailToken, error) {
	if rawToken == "" {
		return model.EmailToken{}, validationError("token is required")
	}
	token, err := s.tokens.GetEmailTokenByHash(ctx, hashEmailToken(rawToken))
	if err != nil || token.Kind != kind {
		return model.EmailToken{}, validationError("token is invalid or expired")
	}
	if token.UsedAt != nil || time.Now().After(token.ExpiresAt) {
		return model.EmailToken{}, validationError("token is invalid or expired")
	}
	if err := s.tokens.MarkEmailTokenUsed(ctx, token.ID); err != nil {
		return model.EmailToken{}, err
	}
	return token, nil
}

func (s *EmailService) newToken(kind, userID string, ttl time.Duration) (string, model.EmailToken) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, model.EmailToken{ID: newID(), UserID: userID, Kind: kind, ExpiresAt: time.Now().Add(ttl), CreatedAt: time.Now()}
}

func hashEmailToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *EmailService) logDebug(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Debug(msg, args...)
	}
}

func (s *EmailService) log(msg string, err error, args ...any) {
	if s.logger == nil {
		return
	}
	if err != nil {
		s.logger.Error(msg, append(args, "err", err)...)
		return
	}
	s.logger.Info(msg, args...)
}
