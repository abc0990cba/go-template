// Login challenges: the bridge between the password step and the code step
// of an MFA login, plus the recovery-code and analytics helpers they share.

package mfa

import (
	"context"
	"crypto/rand"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/users"
)

// emit records a login event attributed to the user when a sink exists.
func (s *Service) emit(ctx context.Context, event, userID string) {
	if s.events != nil {
		s.events.AddEvent(ctx, event, userID)
	}
}

// Authenticate verifies the password step of a login. Three outcomes:
//   - session token set: no MFA, the caller proceeds as before;
//   - challenge token set: the caller must answer the second step and hands
//     the challenge to CompleteChallenge (the response is 401 mfa_required);
//   - ErrEnrollmentRequired: the mfa.required setting blocks accounts
//     without a confirmed factor.
func (s *Service) Authenticate(ctx context.Context, email, password string) (users.User, string, string, error) {
	user, hash, err := s.auth.GetUserByEmail(ctx, strings.ToLower(email))
	if err != nil || !s.verify(hash, password) {
		return users.User{}, "", "", kernel.ErrInvalidCredentials
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
		return users.User{}, "", "", ErrEnrollmentRequired
	}
	token := kernel.NewID()
	if err := s.auth.CreateSession(ctx, token, user.ID, "", s.sessionTTL); err != nil {
		return users.User{}, "", "", err
	}
	s.emit(ctx, "user_logged_in", user.ID)
	return user, token, "", nil
}

// CompleteChallenge finishes an MFA login: the challenge token must be live
// and the code must be a valid TOTP or an unused recovery code.
func (s *Service) CompleteChallenge(ctx context.Context, challengeToken, code string) (users.User, string, error) {
	if challengeToken == "" || code == "" {
		return users.User{}, "", kernel.Invalid("challenge and code are required")
	}
	challenge, err := s.store.GetMFAChallengeByHash(ctx, hashToken(challengeToken))
	if err != nil {
		return users.User{}, "", kernel.Invalid("invalid or expired challenge")
	}
	if time.Now().After(challenge.ExpiresAt) {
		_ = s.store.DeleteMFAChallenge(ctx, challenge.ID)
		return users.User{}, "", kernel.Invalid("invalid or expired challenge")
	}
	if challenge.Attempts >= ChallengeTries {
		_ = s.store.DeleteMFAChallenge(ctx, challenge.ID)
		return users.User{}, "", kernel.Invalid("too many attempts, start again")
	}
	factor, err := s.store.GetMFAFactor(ctx, challenge.UserID)
	if err != nil || !factor.Confirmed {
		return users.User{}, "", kernel.Invalid("invalid or expired challenge")
	}
	if !s.verifyCode(ctx, challenge.UserID, factor, code) {
		_ = s.store.IncrementMFAChallengeAttempts(ctx, challenge.ID)
		return users.User{}, "", kernel.Invalid("invalid code")
	}
	_ = s.store.DeleteMFAChallenge(ctx, challenge.ID)
	token := kernel.NewID()
	if err := s.auth.CreateSession(ctx, token, challenge.UserID, "", s.sessionTTL); err != nil {
		return users.User{}, "", err
	}
	user, err := s.auth.GetUser(ctx, challenge.UserID)
	if err != nil {
		return users.User{}, "", err
	}
	s.emit(ctx, "user_logged_in", user.ID)
	return user, token, nil
}

// verifyCode accepts either a current TOTP or an unused recovery code.
func (s *Service) verifyCode(ctx context.Context, userID string, factor Factor, code string) bool {
	normalized := normalizeCode(code)
	if normalized == "" {
		return false
	}
	if valid, _ := totp.ValidateCustom(normalized, factor.Secret, time.Now(), totp.ValidateOpts{
		Period: uint(TOTPPeriod.Seconds()), Skew: TOTPSkew, Digits: otp.DigitsSix,
	}); valid {
		return true
	}
	if err := s.store.ConsumeMFARecoveryCode(ctx, userID, hashToken(normalized)); err == nil {
		return true
	}
	return false
}

// resetRecoveryCodes generates a fresh set of single-use codes.
func (s *Service) resetRecoveryCodes(ctx context.Context, userID string) ([]string, error) {
	if err := s.store.DeleteMFARecoveryCodes(ctx, userID); err != nil {
		return nil, err
	}
	codes := make([]string, 0, RecoveryCount)
	for range RecoveryCount {
		raw := make([]byte, 5)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		code := base32Lower.EncodeToString(raw)
		if err := s.store.CreateMFARecoveryCode(ctx, RecoveryCode{
			ID: kernel.NewID(), UserID: userID, CodeHash: hashToken(code), CreatedAt: time.Now(),
		}); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, nil
}

func (s *Service) createChallenge(ctx context.Context, userID string) (string, error) {
	token := kernel.NewID()
	now := time.Now()
	if err := s.store.CreateMFAChallenge(ctx, Challenge{
		ID: kernel.NewID(), UserID: userID, TokenHash: hashToken(token), ExpiresAt: now.Add(ChallengeTTL), CreatedAt: now,
	}); err != nil {
		return "", err
	}
	return token, nil
}
