package tokens_test

// The jti denylist: a revoked access token stops verifying immediately,
// other tokens are untouched, and entries expire with the token.

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/your-org/go-template/internal/tokens"
)

func newRevocationService(t *testing.T) (*tokens.Service, *tokens.MemoryRevocations, func(time.Time)) {
	t.Helper()
	svc, _, _ := newTokenService(t)
	revocations := tokens.NewMemoryRevocations()
	base := time.Now()
	revocations.WithClock(func() time.Time { return base })
	svc.WithRevocations(revocations)
	setClock := func(at time.Time) { base = at }
	return svc, revocations, setClock
}

func jtiOf(t *testing.T, token string) (string, time.Time) {
	t.Helper()
	parsed, _, err := new(jwt.Parser).ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	jti, _ := claims["jti"].(string)
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		t.Fatal("token carries no exp")
	}
	return jti, exp.Time
}

func TestRevokeAccessImmediately(t *testing.T) {
	svc, revocations, _ := newRevocationService(t)
	ctx := context.Background()

	_, grant, _, err := svc.PasswordGrant(ctx, "token@example.com", "password123", "ua", "1.2.3.4")
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if _, err := svc.VerifyAccess(grant.AccessToken); err != nil {
		t.Fatalf("verify before revoke: %v", err)
	}

	if err := svc.RevokeAccess(ctx, grant.AccessToken); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.VerifyAccess(grant.AccessToken); err != tokens.ErrInvalidGrant {
		t.Fatalf("revoked token: want ErrInvalidGrant, got %v", err)
	}
	jti, _ := jtiOf(t, grant.AccessToken)
	if revoked, _ := revocations.IsRevokedJTI(ctx, jti); !revoked {
		t.Fatal("denylist entry missing")
	}

	// A different token (new jti) is untouched.
	_, grant2, _, err := svc.PasswordGrant(ctx, "token@example.com", "password123", "ua", "1.2.3.4")
	if err != nil {
		t.Fatalf("second grant: %v", err)
	}
	if _, err := svc.VerifyAccess(grant2.AccessToken); err != nil {
		t.Fatalf("unrelated token must keep working: %v", err)
	}
}

func TestRevokeAccessRejectsGarbage(t *testing.T) {
	svc, _, _ := newRevocationService(t)
	if err := svc.RevokeAccess(context.Background(), "not-a-token"); err != tokens.ErrInvalidGrant {
		t.Fatalf("garbage token: want ErrInvalidGrant, got %v", err)
	}
}

func TestRevocationsExpireWithTheToken(t *testing.T) {
	svc, revocations, setClock := newRevocationService(t)
	ctx := context.Background()

	// Revoke, then walk the denylist clock past the token's exp: the entry
	// must disappear (no unbounded growth).
	_, grant, _, err := svc.PasswordGrant(ctx, "token@example.com", "password123", "ua", "1.2.3.4")
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := svc.RevokeAccess(ctx, grant.AccessToken); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	jti, exp := jtiOf(t, grant.AccessToken)
	setClock(exp.Add(time.Minute))

	revoked, err := revocations.IsRevokedJTI(ctx, jti)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if revoked {
		t.Fatal("expired denylist entry must be dropped")
	}
}
