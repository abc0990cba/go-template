// External test package: the memory store (repository) implements mfa.Store,
// so a same-package test importing it would cycle the import graph.
package mfa_test

import (
	"context"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/mfa"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/settings"
	"github.com/your-org/go-template/internal/users"
)

func newMFAService(t *testing.T) (*mfa.Service, *settings.Service, string) {
	t.Helper()
	store := repository.NewMemoryStore()
	settingsSvc := settings.NewService(store)
	// The service takes its password verifier injected (the argon2 helper
	// lives with the auth feature); the test uses the identity scheme and
	// stores the plain password as the hash.
	svc := mfa.NewService(store, store, settingsSvc, store, 7*24*time.Hour, func(encoded, password string) bool {
		return encoded == password
	})
	ctx := context.Background()
	now := time.Now()
	user := users.User{ID: kernel.NewID(), Email: "mfa@example.com", Name: "Ada", CreatedAt: now, UpdatedAt: now}
	if err := store.CreateUser(ctx, user, "password123"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return svc, settingsSvc, user.ID
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
	svc, _, userID := newMFAService(t)
	ctx := context.Background()

	enabled, pending, _ := svc.Status(ctx, "nope")
	if enabled || pending {
		t.Fatal("unknown user must report disabled")
	}

	if _, _, err := svc.Enroll(ctx, "nope"); err == nil {
		t.Fatal("enroll for unknown user must fail")
	}

	secret, uri, err := svc.Enroll(ctx, userID)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if secret == "" || uri == "" {
		t.Fatal("enroll must return secret and provisioning uri")
	}

	// A wrong code cannot confirm.
	if _, err := svc.VerifyEnrollment(ctx, userID, "000000"); err == nil {
		t.Fatal("wrong code must not confirm")
	}

	codes, err := svc.VerifyEnrollment(ctx, userID, generateTOTP(t, secret))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(codes) != mfa.RecoveryCount {
		t.Fatalf("want %d recovery codes, got %d", mfa.RecoveryCount, len(codes))
	}
	if enabled, _, _ := svc.Status(ctx, userID); !enabled {
		t.Fatal("factor must be confirmed after verify")
	}

	// Re-enrollment is blocked while enabled.
	if _, _, err := svc.Enroll(ctx, userID); err == nil {
		t.Fatal("re-enroll while enabled must fail")
	}

	// Disable requires password and a valid code.
	if err := svc.Disable(ctx, userID, "wrong-password", generateTOTP(t, secret)); err == nil {
		t.Fatal("disable with wrong password must fail")
	}
	if err := svc.Disable(ctx, userID, "password123", generateTOTP(t, secret)); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if enabled, _, _ := svc.Status(ctx, userID); enabled {
		t.Fatal("factor must be gone after disable")
	}
}

func TestMFALoginChallengeFlow(t *testing.T) {
	svc, _, userID := newMFAService(t)
	ctx := context.Background()
	secret, _, err := svc.Enroll(ctx, userID)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if _, err := svc.VerifyEnrollment(ctx, userID, generateTOTP(t, secret)); err != nil {
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
	for range mfa.ChallengeTries {
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
	if token == "" || user2.ID != userID {
		t.Fatal("complete must open a session for the user")
	}

	// ...or with an unused recovery code, which then cannot repeat
	// (covered end to end by TestMFARecoveryCodeSingleUse).
	_, _, challenge, _ = svc.Authenticate(ctx, "mfa@example.com", "password123")
	if _, _, err := svc.CompleteChallenge(ctx, challenge, "000000"); err == nil {
		t.Fatal("bogus code must fail")
	}
}

func TestMFARequiredSettingBlocksUnenrolled(t *testing.T) {
	svc, settingsSvc, _ := newMFAService(t)
	ctx := context.Background()
	if err := settingsSvc.Set(ctx, "mfa.required", "true", "admin"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	_, _, _, err := svc.Authenticate(ctx, "mfa@example.com", "password123")
	if err != mfa.ErrEnrollmentRequired {
		t.Fatalf("want ErrEnrollmentRequired, got %v", err)
	}
	// Wrong password still reports invalid credentials, not the MFA error.
	if _, _, _, err := svc.Authenticate(ctx, "mfa@example.com", "nope"); err != kernel.ErrInvalidCredentials {
		t.Fatalf("want kernel.ErrInvalidCredentials, got %v", err)
	}
}

func TestMFARecoveryCodeSingleUse(t *testing.T) {
	svc, _, userID := newMFAService(t)
	ctx := context.Background()
	secret, _, _ := svc.Enroll(ctx, userID)
	codes, err := svc.VerifyEnrollment(ctx, userID, generateTOTP(t, secret))
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
