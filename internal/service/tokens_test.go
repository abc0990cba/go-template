package service

import (
	"context"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/your-org/go-template/internal/repository"
)

func totpCode(secret string) (string, error) {
	return totp.GenerateCode(secret, time.Now())
}

const testJWTSecret = "unit-test-secret-0123456789abcdef"

func newTokenService(t *testing.T) (*TokenService, *repository.MemoryStore) {
	t.Helper()
	store := repository.NewMemoryStore()
	auth := &AuthService{store: store, sessionTTL: 7 * 24 * time.Hour}
	settings := NewSettingsService(store)
	mfa := NewMFAService(store, store, settings, store, 7*24*time.Hour)
	if _, _, err := auth.Register(context.Background(), "token@example.com", "Ada", "password123"); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := NewTokenService(store, store, mfa, store, testJWTSecret, 15*time.Minute, 24*time.Hour)
	return svc, store
}

func TestPasswordGrantIssuesTokens(t *testing.T) {
	svc, _ := newTokenService(t)
	ctx := context.Background()

	user, grant, challenge, err := svc.PasswordGrant(ctx, "token@example.com", "password123", "ua", "1.2.3.4")
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if challenge != "" || grant == nil || grant.AccessToken == "" || grant.RefreshToken == "" {
		t.Fatalf("want full grant, got challenge=%q grant=%v", challenge, grant)
	}
	if user.Email != "token@example.com" {
		t.Fatalf("wrong user: %v", user.Email)
	}

	// The access token authenticates API calls.
	sub, err := svc.VerifyAccess(grant.AccessToken)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if sub != user.ID {
		t.Fatalf("sub mismatch: %q", sub)
	}

	// Wrong passwords stay uniform.
	if _, _, _, err := svc.PasswordGrant(ctx, "token@example.com", "nope", "ua", "1.2.3.4"); err != ErrInvalidGrant {
		t.Fatalf("want ErrInvalidGrant, got %v", err)
	}
}

func TestRefreshRotationAndReuseDetection(t *testing.T) {
	svc, _ := newTokenService(t)
	ctx := context.Background()

	_, first, _, err := svc.PasswordGrant(ctx, "token@example.com", "password123", "ua", "1.2.3.4")
	if err != nil {
		t.Fatalf("grant: %v", err)
	}

	// Rotation: the refresh token works once and returns a fresh pair.
	_, second, err := svc.RefreshGrant(ctx, first.RefreshToken, "ua", "1.2.3.4")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh must rotate the token")
	}

	// Reuse of the rotated token kills the whole family.
	if _, _, err := svc.RefreshGrant(ctx, first.RefreshToken, "ua", "1.2.3.4"); err != ErrTokenReuse {
		t.Fatalf("want ErrTokenReuse, got %v", err)
	}
	if _, _, err := svc.RefreshGrant(ctx, second.RefreshToken, "ua", "1.2.3.4"); err != ErrTokenReuse {
		t.Fatalf("family token must be dead after reuse, got %v", err)
	}
}

func TestRevokeAndRevokeAll(t *testing.T) {
	svc, _ := newTokenService(t)
	ctx := context.Background()

	_, first, _, _ := svc.PasswordGrant(ctx, "token@example.com", "password123", "ua", "ip")
	_, second, _, _ := svc.PasswordGrant(ctx, "token@example.com", "password123", "ua", "ip")

	if err := svc.Revoke(ctx, first.RefreshToken); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, _, err := svc.RefreshGrant(ctx, first.RefreshToken, "ua", "ip"); err == nil {
		t.Fatal("revoked token must not refresh")
	}
	// Unknown tokens revoke silently (no oracle).
	if err := svc.Revoke(ctx, "does-not-exist"); err != nil {
		t.Fatalf("unknown revoke must be silent: %v", err)
	}

	if err := svc.RevokeAllForUser(ctx, "someone-else"); err != nil {
		t.Fatalf("revoke all: %v", err)
	}
	if _, _, err := svc.RefreshGrant(ctx, second.RefreshToken, "ua", "ip"); err != nil {
		t.Fatalf("other family untouched: %v", err)
	}
}

func TestTokenGrantWithoutSecretDisabled(t *testing.T) {
	store := repository.NewMemoryStore()
	svc := NewTokenService(store, store, nil, nil, "", time.Minute, time.Hour)
	if _, _, _, err := svc.PasswordGrant(context.Background(), "a@b.c", "x", "", ""); err != ErrTokenAuthDisabled {
		t.Fatalf("want ErrTokenAuthDisabled, got %v", err)
	}
	if _, err := svc.VerifyAccess("whatever"); err != ErrTokenAuthDisabled {
		t.Fatalf("verify must be disabled too, got %v", err)
	}
}

func TestAccessTokenRejectsTampering(t *testing.T) {
	svc, _ := newTokenService(t)
	_, grant, _, _ := svc.PasswordGrant(context.Background(), "token@example.com", "password123", "ua", "ip")

	// A different secret must not verify.
	other := NewTokenService(repository.NewMemoryStore(), nil, nil, nil, "another-secret-0123456789abcdef", time.Minute, time.Hour)
	if _, err := other.VerifyAccess(grant.AccessToken); err == nil {
		t.Fatal("foreign secret must not verify")
	}
	if _, err := svc.VerifyAccess(grant.AccessToken + "x"); err == nil {
		t.Fatal("tampered token must not verify")
	}
}

func TestPasswordGrantMFAChallengeBridge(t *testing.T) {
	svc, _ := newTokenService(t)
	ctx := context.Background()

	// Enroll MFA for the token user through the MFA service.
	user, _, _ := svc.auth.GetUserByEmail(ctx, "token@example.com")
	secret, _, err := svc.mfa.Enroll(ctx, user.ID)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	code, err := totpCode(secret)
	if err != nil {
		t.Fatalf("code: %v", err)
	}
	if _, err := svc.mfa.VerifyEnrollment(ctx, user.ID, code); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// The grant yields a challenge instead of tokens.
	_, grant, challenge, err := svc.PasswordGrant(ctx, "token@example.com", "password123", "ua", "ip")
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if grant != nil || challenge == "" {
		t.Fatalf("want challenge-only grant, got grant=%v challenge=%q", grant, challenge)
	}

	// The challenge grant completes.
	code, _ = totpCode(secret)
	user2, grant2, err := svc.CompleteChallengeGrant(ctx, challenge, code, "ua", "ip")
	if err != nil {
		t.Fatalf("challenge grant: %v", err)
	}
	if grant2 == nil || user2.ID != user.ID {
		t.Fatal("challenge grant must issue tokens")
	}
}
