// Package mfa implements TOTP two-factor authentication: enrollment with a
// provisioning URI, login challenges that bridge the password step and the
// code step, and hashed single-use recovery codes.
package mfa

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/users"
)

// ErrEnrollmentRequired is returned by Authenticate when the mfa.required
// setting is on and the account has no confirmed factor yet. The handler
// maps it to 403 with a machine-readable marker so the UI can route to the
// enrollment page.
var ErrEnrollmentRequired = errors.New("mfa enrollment required")

// MFA tuning.
const (
	ChallengeTTL   = 5 * time.Minute
	ChallengeTries = 5
	RecoveryCount  = 10
	Issuer         = "go-template"
	TOTPPeriod     = 30 * time.Second
	TOTPSkew       = 1
)

// Auth is the slice of the credential store the two-factor flow needs:
// verifying credentials, opening the post-challenge session, and reading
// users. Password verification itself rides the verify constructor function
// (the argon2 helper lives with the auth feature).
type Auth interface {
	GetUser(ctx context.Context, id string) (users.User, error)
	GetUserByEmail(ctx context.Context, email string) (users.User, string, error)
	CreateSession(ctx context.Context, token, userID, impersonatedBy string, ttl time.Duration) error
}

// Settings reads the feature flags (mfa.required); *settings.Service
// satisfies it.
type Settings interface {
	Bool(ctx context.Context, key string) bool
}

// Store persists TOTP factors, hashed recovery codes, and short-lived
// login challenges. Secrets and codes are stored as the service produced
// them (base32 secret, sha256 code hashes); the raw values never leave it.
type Store interface {
	UpsertMFAFactor(ctx context.Context, factor Factor) error
	ConfirmMFAFactor(ctx context.Context, userID string) error
	GetMFAFactor(ctx context.Context, userID string) (Factor, error)
	DeleteMFAFactor(ctx context.Context, userID string) error

	CreateMFARecoveryCode(ctx context.Context, code RecoveryCode) error
	ListMFARecoveryCodes(ctx context.Context, userID string) ([]RecoveryCode, error)
	ConsumeMFARecoveryCode(ctx context.Context, userID, codeHash string) error
	DeleteMFARecoveryCodes(ctx context.Context, userID string) error

	CreateMFAChallenge(ctx context.Context, challenge Challenge) error
	GetMFAChallengeByHash(ctx context.Context, tokenHash string) (Challenge, error)
	IncrementMFAChallengeAttempts(ctx context.Context, id string) error
	DeleteMFAChallenge(ctx context.Context, id string) error
	DeleteExpiredMFAChallenges(ctx context.Context) error
}

// Factor is the user's TOTP enrollment. The secret stays server-side;
// confirmed flips once the user proved they can generate valid codes.
type Factor struct {
	UserID      string     `json:"user_id"`
	Secret      string     `json:"-"`
	Confirmed   bool       `json:"confirmed"`
	CreatedAt   time.Time  `json:"created_at"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// RecoveryCode is one hashed single-use backup code. The raw code is
// shown exactly once at enrollment.
type RecoveryCode struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	CodeHash  string     `json:"-"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// Challenge bridges the password step and the TOTP step of a login. The
// token is handed to the client once; only its hash is stored.
type Challenge struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TokenHash string    `json:"-"`
	Attempts  int       `json:"attempts"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// Service implements the TOTP flows: enrollment, login challenges, and
// hashed single-use recovery codes.
type Service struct {
	store      Store
	auth       Auth
	settings   Settings
	events     analytics.Sink
	sessionTTL time.Duration
	// verify checks a password against its stored hash; injected because
	// the argon2 helper lives with the auth feature.
	verify func(encoded, password string) bool
}

// NewService wires the service. auth is used to verify credentials and to
// open the post-challenge session, so the login flow changes in one place.
// events is the analytics trail; nil is valid in processes without a sink.
func NewService(store Store, auth Auth, settings Settings, events analytics.Sink, sessionTTL time.Duration, verify func(encoded, password string) bool) *Service {
	return &Service{store: store, auth: auth, settings: settings, events: events, sessionTTL: sessionTTL, verify: verify}
}

// Enroll starts TOTP enrollment: it stores an unconfirmed factor and returns
// the base32 secret plus the otpauth:// provisioning URI (the SPA renders
// the QR from the URI). A confirmed factor blocks re-enrollment — disable
// first.
func (s *Service) Enroll(ctx context.Context, userID string) (secret, url string, err error) {
	if factor, ferr := s.store.GetMFAFactor(ctx, userID); ferr == nil && factor.Confirmed {
		return "", "", fmt.Errorf("%w: two-factor authentication is already enabled", kernel.ErrConflict)
	}
	user, err := s.auth.GetUser(ctx, userID)
	if err != nil {
		return "", "", err
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      Issuer,
		AccountName: user.Email,
		Period:      uint(TOTPPeriod.Seconds()),
		Digits:      otp.DigitsSix,
	})
	if err != nil {
		return "", "", fmt.Errorf("generate totp key: %w", err)
	}
	now := time.Now()
	if err := s.store.UpsertMFAFactor(ctx, Factor{UserID: userID, Secret: key.Secret(), CreatedAt: now, UpdatedAt: now}); err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// VerifyEnrollment confirms the factor once the user proved they can
// produce valid codes, replaces the recovery codes, and returns the fresh
// codes — shown exactly once.
func (s *Service) VerifyEnrollment(ctx context.Context, userID, code string) ([]string, error) {
	factor, err := s.store.GetMFAFactor(ctx, userID)
	if err != nil {
		return nil, kernel.Invalid("no pending enrollment")
	}
	if factor.Confirmed {
		return nil, kernel.Invalid("two-factor authentication is already enabled")
	}
	valid, _ := totp.ValidateCustom(normalizeCode(code), factor.Secret, time.Now(), totp.ValidateOpts{
		Period: uint(TOTPPeriod.Seconds()), Skew: TOTPSkew, Digits: otp.DigitsSix,
	})
	if !valid {
		return nil, kernel.Invalid("invalid code")
	}
	if err := s.store.ConfirmMFAFactor(ctx, userID); err != nil {
		return nil, err
	}
	return s.resetRecoveryCodes(ctx, userID)
}

// Disable turns MFA off. Both the password and a current valid code are
// required, so a stolen session alone cannot weaken the account.
func (s *Service) Disable(ctx context.Context, userID, password, code string) error {
	_, hash, err := s.auth.GetUserByEmail(ctx, s.email(ctx, userID))
	if err != nil || hash == "" || !s.verify(hash, password) {
		return kernel.ErrInvalidCredentials
	}
	factor, err := s.store.GetMFAFactor(ctx, userID)
	if err != nil {
		return kernel.Invalid("two-factor authentication is not enabled")
	}
	if !s.verifyCode(ctx, userID, factor, code) {
		return kernel.Invalid("invalid code")
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
func (s *Service) Reset(ctx context.Context, userID string) error {
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
func (s *Service) Status(ctx context.Context, userID string) (enabled bool, pending bool, err error) {
	factor, err := s.store.GetMFAFactor(ctx, userID)
	if err != nil {
		return false, false, nil // no factor is the normal state
	}
	return factor.Confirmed, !factor.Confirmed, nil
}

// Enabled answers whether the user has a confirmed factor; the login flow
// and (later) the token grant use it.
func (s *Service) Enabled(ctx context.Context, userID string) bool {
	factor, err := s.store.GetMFAFactor(ctx, userID)
	return err == nil && factor.Confirmed
}

// PruneChallenges drops expired login challenges; the worker calls it on a
// schedule like the idempotency pruner.
func (s *Service) PruneChallenges(ctx context.Context) error {
	return s.store.DeleteExpiredMFAChallenges(ctx)
}

// BeginChallenge mints a login challenge for the token grant (exported form
// of createChallenge).
func (s *Service) BeginChallenge(ctx context.Context, userID string) (string, error) {
	return s.createChallenge(ctx, userID)
}

// RequiredAndMissing reports whether the mfa.required policy demands a
// factor the account does not have confirmed yet.
func (s *Service) RequiredAndMissing(ctx context.Context, userID string) bool {
	if s.settings == nil || !s.settings.Bool(ctx, "mfa.required") {
		return false
	}
	factor, err := s.store.GetMFAFactor(ctx, userID)
	return !(err == nil && factor.Confirmed)
}

func (s *Service) email(ctx context.Context, userID string) string {
	user, err := s.auth.GetUser(ctx, userID)
	if err != nil {
		return ""
	}
	return user.Email
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
// (sha256 hex), mirroring the email-token scheme. Copied in the service
// package for refresh/invitation tokens — a little copying beats a shared
// helper dependency.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// base32Lower encodes recovery-code entropy as lowercase base32 without
// padding, so codes contain only letters and digits.
var base32Lower = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)
