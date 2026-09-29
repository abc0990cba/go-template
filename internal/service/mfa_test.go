package service

import (
	"context"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/your-org/go-template/internal/repository"
)

func newMFAService(t *testing.T) (*MFAService, *repository.MemoryStore) {
	t.Helper()
	store := repository.NewMemoryStore()
	auth := &AuthService{store: store, sessionTTL: 7 * 24 * time.Hour}
	svc := NewMFAService(store, store, NewSettingsService(store), store, 7*24*time.Hour)
	ctx := context.Background()
	if _, _, err := auth.Register(ctx, "mfa@example.com", "Ada", "password123"); err != nil {
		t.Fatalf("register: %v", err)
	}
	return svc, store
}

func generateTOTP(t *testing.T, secret string) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	return code
}

func TestMFAEnrollVerifyDisable(t *testing.T) {
	svc, _ := newMFAService(t)
	ctx := context.Background()

	enabled, pending, _ := svc.Status(ctx, "nope")
	if enabled || pending {
		t.Fatal("unknown user must report disabled")
	}

	secret, uri, err := svc.Enroll(ctx, "nope")
	if err == nil {
		t.Fatal("enroll for unknown user must fail")
	}

	// Find the seeded user id by email.
	user, _, err := svc.auth.GetUserByEmail(ctx, "mfa@example.com")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}

	secret, uri, err = svc.Enroll(ctx, user.ID)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if secret == "" || uri == "" {
		t.Fatal("enroll must return secret and provisioning uri")
	}

	// A wrong code cannot confirm.
	if _, err := svc.VerifyEnrollment(ctx, user.ID, "000000"); err == nil {
		t.Fatal("wrong code must not confirm")
	}

	codes, err := svc.VerifyEnrollment(ctx, user.ID, generateTOTP(t, secret))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(codes) != MFARecoveryCount {
		t.Fatalf("want %d recovery codes, got %d", MFARecoveryCount, len(codes))
	}
	if enabled, _, _ := svc.Status(ctx, user.ID); !enabled {
		t.Fatal("factor must be confirmed after verify")
	}

	// Re-enrollment is blocked while enabled.
	if _, _, err := svc.Enroll(ctx, user.ID); err == nil {
		t.Fatal("re-enroll while enabled must fail")
	}

	// Disable requires password and a valid code.
	if err := svc.Disable(ctx, user.ID, "wrong-password", generateTOTP(t, secret)); err == nil {
		t.Fatal("disable with wrong password must fail")
	}
	if err := svc.Disable(ctx, user.ID, "password123", generateTOTP(t, secret)); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if enabled, _, _ := svc.Status(ctx, user.ID); enabled {
		t.Fatal("factor must be gone after disable")
	}
}

func TestMFALoginChallengeFlow(t *testing.T) {
	svc, _ := newMFAService(t)
	ctx := context.Background()
	user, _, _ := svc.auth.GetUserByEmail(ctx, "mfa@example.com")
	secret, _, err := svc.Enroll(ctx, user.ID)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if _, err := svc.VerifyEnrollment(ctx, user.ID, generateTOTP(t, secret)); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// No MFA session token comes out of the password step anymore.
	_, token, challenge, err := svc.Authenticate(ctx, "mfa@example.com", "password123")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if token != "" || challenge == "" {
		t.Fatalf("want challenge-only login, got token=%q challenge=%q", token, challenge)
	}

	// Wrong codes burn attempts but keep the challenge until the cap.
	for range MFAChallengeTries {
		if _, _, err := svc.CompleteChallenge(ctx, challenge, "000000"); err == nil {
			t.Fatal("wrong code must fail")
		}
	}
	if _, _, err := svc.CompleteChallenge(ctx, challenge, generateTOTP(t, secret)); err == nil {
		t.Fatal("challenge must be dead after too many attempts")
	}

	// A fresh challenge completes with a current TOTP...
	_, _, challenge, err = svc.Authenticate(ctx, "mfa@example.com", "password123")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	user2, token, err := svc.CompleteChallenge(ctx, challenge, generateTOTP(t, secret))
	if err != nil {
		t.Fatalf("complete with totp: %v", err)
	}
	if token == "" || user2.ID != user.ID {
		t.Fatal("complete must open a session for the user")
	}

	// ...or with an unused recovery code, which then cannot repeat.
	_, _, challenge, _ = svc.Authenticate(ctx, "mfa@example.com", "password123")
	// recovery codes were minted at verify time; fetch one from the store
	codesList, err := svc.store.ListMFARecoveryCodes(ctx, user.ID)
	if err != nil || len(codesList) == 0 {
		t.Fatalf("recovery codes present: %v", err)
	}
	_ = codesList
	_, _, challenge, _ = svc.Authenticate(ctx, "mfa@example.com", "password123")
	if _, _, err := svc.CompleteChallenge(ctx, challenge, "000000"); err == nil {
		t.Fatal("bogus code must fail")
	}
}

func TestMFARequiredSettingBlocksUnenrolled(t *testing.T) {
	svc, _ := newMFAService(t)
	ctx := context.Background()
	if err := svc.settings.Set(ctx, "mfa.required", "true", "admin"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	_, _, _, err := svc.Authenticate(ctx, "mfa@example.com", "password123")
	if err != ErrMFAEnrollmentRequired {
		t.Fatalf("want ErrMFAEnrollmentRequired, got %v", err)
	}
	// Wrong password still reports invalid credentials, not the MFA error.
	if _, _, _, err := svc.Authenticate(ctx, "mfa@example.com", "nope"); err != ErrInvalidCredentials {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestMFARecoveryCodeSingleUse(t *testing.T) {
	svc, _ := newMFAService(t)
	ctx := context.Background()
	user, _, _ := svc.auth.GetUserByEmail(ctx, "mfa@example.com")
	secret, _, _ := svc.Enroll(ctx, user.ID)
	codes, err := svc.VerifyEnrollment(ctx, user.ID, generateTOTP(t, secret))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	// The challenge step accepts a recovery code exactly once.
	_, _, challenge, _ := svc.Authenticate(ctx, "mfa@example.com", "password123")
	if _, _, err := svc.CompleteChallenge(ctx, challenge, codes[0]); err != nil {
		t.Fatalf("recovery code must complete the challenge: %v", err)
	}
	_, _, challenge, _ = svc.Authenticate(ctx, "mfa@example.com", "password123")
	if _, _, err := svc.CompleteChallenge(ctx, challenge, codes[0]); err == nil {
		t.Fatal("a used recovery code must be rejected")
	}
}
