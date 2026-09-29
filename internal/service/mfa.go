package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
)

// ErrMFAEnrollmentRequired is returned by Authenticate when the mfa.required
// setting is on and the account has no confirmed factor yet. The handler
// maps it to 403 with a machine-readable marker so the UI can route to the
// enrollment page.
var ErrMFAEnrollmentRequired = errors.New("mfa enrollment required")

// MFA tuning.
const (
	MFAChallengeTTL   = 5 * time.Minute
	MFAChallengeTries = 5
	MFARecoveryCount  = 10
	MFAIssuer         = "go-template"
	MFATOTPPeriod     = 30 * time.Second
	MFATOTPSkew       = 1
)

// MFAService implements TOTP two-factor authentication: enrollment with a
// provisioning URI, login challenges that bridge the password step and the
// code step, and hashed single-use recovery codes.
type MFAService struct {
	store      MFARepo
	auth       AuthRepo
	settings   *SettingsService
	events     EventSink
	sessionTTL time.Duration
}

// NewMFAService wires the service. auth is used to verify credentials and to
// open the post-challenge session, so the login flow changes in one place.
// events is the analytics trail; nil is valid in processes without a sink.
func NewMFAService(store MFARepo, auth AuthRepo, settings *SettingsService, events EventSink, sessionTTL time.Duration) *MFAService {
	return &MFAService{store: store, auth: auth, settings: settings, events: events, sessionTTL: sessionTTL}
}

// emit records a login event attributed to the user when a sink exists.
func (s *MFAService) emit(ctx context.Context, event, userID string) {
	if s.events != nil {
		s.events.AddEvent(ctx, event, userID)
	}
}

// Authenticate verifies the password step of a login. Three outcomes:
//   - session token set: no MFA, the caller proceeds as before;
//   - challenge token set: the caller must answer the second step and hands
//     the challenge to CompleteChallenge (the response is 401 mfa_required);
//   - ErrMFAEnrollmentRequired: the mfa.required setting blocks accounts
//     without a confirmed factor.
func (s *MFAService) Authenticate(ctx context.Context, email, password string) (model.User, string, string, error) {
	user, hash, err := s.auth.GetUserByEmail(ctx, strings.ToLower(email))
	if err != nil || !verifyPassword(hash, password) {
		return model.User{}, "", "", ErrInvalidCredentials
	}
	factor, err := s.store.GetMFAFactor(ctx, user.ID)
	confirmed := err == nil && factor.Confirmed
	switch {
	case confirmed:
		token, cerr := s.createChallenge(ctx, user.ID)
		return user, "", token, cerr
	case s.settings != nil && s.settings.Bool(ctx, "mfa.required"):
		// A pending (unconfirmed) enrollment does not satisfy the policy:
		// the factor must be proven before login is allowed.
		return model.User{}, "", "", ErrMFAEnrollmentRequired
	}
	token := newID()
	if err := s.auth.CreateSession(ctx, token, user.ID, "", s.sessionTTL); err != nil {
		return model.User{}, "", "", err
	}
	s.emit(ctx, "user_logged_in", user.ID)
	return user, token, "", nil
}

// CompleteChallenge finishes an MFA login: the challenge token must be live
// and the code must be a valid TOTP or an unused recovery code.
func (s *MFAService) CompleteChallenge(ctx context.Context, challengeToken, code string) (model.User, string, error) {
	if challengeToken == "" || code == "" {
		return model.User{}, "", validationError("challenge and code are required")
	}
	challenge, err := s.store.GetMFAChallengeByHash(ctx, hashToken(challengeToken))
	if err != nil {
		return model.User{}, "", validationError("invalid or expired challenge")
	}
	if time.Now().After(challenge.ExpiresAt) {
		_ = s.store.DeleteMFAChallenge(ctx, challenge.ID)
		return model.User{}, "", validationError("invalid or expired challenge")
	}
	if challenge.Attempts >= MFAChallengeTries {
		_ = s.store.DeleteMFAChallenge(ctx, challenge.ID)
		return model.User{}, "", validationError("too many attempts, start again")
	}
	factor, err := s.store.GetMFAFactor(ctx, challenge.UserID)
	if err != nil || !factor.Confirmed {
		return model.User{}, "", validationError("invalid or expired challenge")
	}
	if !s.verifyCode(ctx, challenge.UserID, factor, code) {
		_ = s.store.IncrementMFAChallengeAttempts(ctx, challenge.ID)
		return model.User{}, "", validationError("invalid code")
	}
	_ = s.store.DeleteMFAChallenge(ctx, challenge.ID)
	token := newID()
	if err := s.auth.CreateSession(ctx, token, challenge.UserID, "", s.sessionTTL); err != nil {
		return model.User{}, "", err
	}
	user, err := s.auth.GetUser(ctx, challenge.UserID)
	if err != nil {
		return model.User{}, "", err
	}
	s.emit(ctx, "user_logged_in", user.ID)
	return user, token, nil
}

// Enroll starts TOTP enrollment: it stores an unconfirmed factor and returns
// the base32 secret plus the otpauth:// provisioning URI (the SPA renders
// the QR from the URI). A confirmed factor blocks re-enrollment — disable
// first.
func (s *MFAService) Enroll(ctx context.Context, userID string) (secret, url string, err error) {
	if factor, ferr := s.store.GetMFAFactor(ctx, userID); ferr == nil && factor.Confirmed {
		return "", "", fmt.Errorf("%w: two-factor authentication is already enabled", repository.ErrConflict)
	}
	user, err := s.auth.GetUser(ctx, userID)
	if err != nil {
		return "", "", err
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      MFAIssuer,
		AccountName: user.Email,
		Period:      uint(MFATOTPPeriod.Seconds()),
		Digits:      otp.DigitsSix,
	})
	if err != nil {
		return "", "", fmt.Errorf("generate totp key: %w", err)
	}
	now := time.Now()
	if err := s.store.UpsertMFAFactor(ctx, model.MFAFactor{UserID: userID, Secret: key.Secret(), CreatedAt: now, UpdatedAt: now}); err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// VerifyEnrollment confirms the factor once the user proved they can
// produce valid codes, replaces the recovery codes, and returns the fresh
// codes — shown exactly once.
func (s *MFAService) VerifyEnrollment(ctx context.Context, userID, code string) ([]string, error) {
	factor, err := s.store.GetMFAFactor(ctx, userID)
	if err != nil {
		return nil, validationError("no pending enrollment")
	}
	if factor.Confirmed {
		return nil, validationError("two-factor authentication is already enabled")
	}
	valid, _ := totp.ValidateCustom(normalizeCode(code), factor.Secret, time.Now(), totp.ValidateOpts{
		Period: uint(MFATOTPPeriod.Seconds()), Skew: MFATOTPSkew, Digits: otp.DigitsSix,
	})
	if !valid {
		return nil, validationError("invalid code")
	}
	if err := s.store.ConfirmMFAFactor(ctx, userID); err != nil {
		return nil, err
	}
	return s.resetRecoveryCodes(ctx, userID)
}

// Disable turns MFA off. Both the password and a current valid code are
// required, so a stolen session alone cannot weaken the account.
func (s *MFAService) Disable(ctx context.Context, userID, password, code string) error {
	_, hash, err := s.auth.GetUserByEmail(ctx, s.email(ctx, userID))
	if err != nil || hash == "" || !verifyPassword(hash, password) {
		return ErrInvalidCredentials
	}
	factor, err := s.store.GetMFAFactor(ctx, userID)
	if err != nil {
		return validationError("two-factor authentication is not enabled")
	}
	if !s.verifyCode(ctx, userID, factor, code) {
		return validationError("invalid code")
	}
	if err := s.store.DeleteMFAFactor(ctx, userID); err != nil {
		return err
	}
	return s.store.DeleteMFARecoveryCodes(ctx, userID)
}

// Reset wipes a user's factor and recovery codes on admin order (audited by
// the handler); the user re-enrolls from their profile. It is idempotent —
// resetting an account without a factor is a no-op success so the admin
// console does not need to probe first.
func (s *MFAService) Reset(ctx context.Context, userID string) error {
	if _, err := s.auth.GetUser(ctx, userID); err != nil {
		return err
	}
	if err := s.store.DeleteMFAFactor(ctx, userID); err != nil {
		return err
	}
	if err := s.store.DeleteMFARecoveryCodes(ctx, userID); err != nil {
		return err
	}
	return s.store.DeleteExpiredMFAChallenges(ctx)
}

// Status reports the enrollment state for the profile page.
func (s *MFAService) Status(ctx context.Context, userID string) (enabled bool, pending bool, err error) {
	factor, err := s.store.GetMFAFactor(ctx, userID)
	if err != nil {
		return false, false, nil // no factor is the normal state
	}
	return factor.Confirmed, !factor.Confirmed, nil
}

// Enabled answers whether the user has a confirmed factor; the login flow
// and (later) the token grant use it.
func (s *MFAService) Enabled(ctx context.Context, userID string) bool {
	factor, err := s.store.GetMFAFactor(ctx, userID)
	return err == nil && factor.Confirmed
}

// PruneChallenges drops expired login challenges; the worker calls it on a
// schedule like the idempotency pruner.
func (s *MFAService) PruneChallenges(ctx context.Context) error {
	return s.store.DeleteExpiredMFAChallenges(ctx)
}

// verifyCode accepts either a current TOTP or an unused recovery code.
func (s *MFAService) verifyCode(ctx context.Context, userID string, factor model.MFAFactor, code string) bool {
	normalized := normalizeCode(code)
	if normalized == "" {
		return false
	}
	if valid, _ := totp.ValidateCustom(normalized, factor.Secret, time.Now(), totp.ValidateOpts{
		Period: uint(MFATOTPPeriod.Seconds()), Skew: MFATOTPSkew, Digits: otp.DigitsSix,
	}); valid {
		return true
	}
	if err := s.store.ConsumeMFARecoveryCode(ctx, userID, hashToken(normalized)); err == nil {
		return true
	}
	return false
}

// resetRecoveryCodes generates a fresh set of single-use codes.
func (s *MFAService) resetRecoveryCodes(ctx context.Context, userID string) ([]string, error) {
	if err := s.store.DeleteMFARecoveryCodes(ctx, userID); err != nil {
		return nil, err
	}
	codes := make([]string, 0, MFARecoveryCount)
	for range MFARecoveryCount {
		raw := make([]byte, 5)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		code := base32Lower.EncodeToString(raw)
		if err := s.store.CreateMFARecoveryCode(ctx, model.MFARecoveryCode{
			ID: newID(), UserID: userID, CodeHash: hashToken(code), CreatedAt: time.Now(),
		}); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, nil
}

func (s *MFAService) email(ctx context.Context, userID string) string {
	user, err := s.auth.GetUser(ctx, userID)
	if err != nil {
		return ""
	}
	return user.Email
}

func (s *MFAService) createChallenge(ctx context.Context, userID string) (string, error) {
	token := newID()
	now := time.Now()
	if err := s.store.CreateMFAChallenge(ctx, model.MFAChallenge{
		ID: newID(), UserID: userID, TokenHash: hashToken(token), ExpiresAt: now.Add(MFAChallengeTTL), CreatedAt: now,
	}); err != nil {
		return "", err
	}
	return token, nil
}

// normalizeCode strips the formatting humans paste (dashes, spaces) and
// lowercases; TOTP digits pass through untouched.
func normalizeCode(code string) string {
	var b strings.Builder
	for _, r := range code {
		switch {
		case r == '-' || r == ' ':
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// hashToken is the one place challenge tokens and recovery codes are hashed
// (sha256 hex), mirroring the email-token scheme.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// base32Lower encodes recovery-code entropy as lowercase base32 without
// padding, so codes contain only letters and digits.
var base32Lower = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)
