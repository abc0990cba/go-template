package tokens_test

// kid-based secret rotation: new tokens carry the active kid, every listed
// kid keeps verifying, kid-less legacy tokens verify against APP_JWT_SECRET,
// and tokens signed by an attacker are rejected regardless of kid.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/tokens"
)

const (
	legacySecret = "legacy-single-secret-that-is-long-enough"
	rotOldKey    = "first-key-that-is-long-enough-32b!"
	rotNewKey    = "second-key-that-is-long-enough-32!"
)

// keySetFor parses a keyset with the legacy secret configured.
func keySetFor(t *testing.T, active string, keys map[string]string) *tokens.KeySet {
	t.Helper()
	return mustKeySet(t, keySetJSON(t, active, keys), legacySecret)
}

// mustKeySet parses raw keyset file contents, failing the test on error.
func mustKeySet(t *testing.T, data []byte, legacy string) *tokens.KeySet {
	t.Helper()
	set, err := tokens.ParseKeySet(data, legacy)
	if err != nil {
		t.Fatalf("parse keyset: %v", err)
	}
	return set
}

func keySetJSON(t *testing.T, active string, keys map[string]string) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{"active": active, "keys": keys})
	if err != nil {
		t.Fatalf("marshal keyset: %v", err)
	}
	return data
}

func TestKeySetParsing(t *testing.T) {
	set := keySetFor(t, "k2", map[string]string{"k1": rotOldKey, "k2": rotNewKey})
	kid, secret := set.Active()
	if kid != "k2" || len(secret) == 0 {
		t.Fatalf("active: kid=%q", kid)
	}
	if _, ok := set.Get("k1"); !ok {
		t.Fatal("old key must stay resolvable during rotation")
	}
	kids := set.Kids()
	if len(kids) != 2 || kids[0] != "k1" || kids[1] != "k2" {
		t.Fatalf("kids: %v", kids)
	}

	// Validation floors and required fields.
	longEnough := "key-that-is-at-least-32-bytes!!"
	bad := []struct {
		name string
		data string
	}{
		{"short secret", `{"active":"a","keys":{"a":"short"}}`},
		{"active missing from list", `{"active":"gone","keys":{"a":"` + longEnough + `"}}`},
		{"no active", `{"keys":{"a":"` + longEnough + `"}}`},
		{"no keys", `{"active":"a","keys":{}}`},
	}
	for _, tc := range bad {
		if _, err := tokens.ParseKeySet([]byte(tc.data), ""); err == nil {
			t.Fatalf("%s: want error", tc.name)
		}
	}
}

func TestRotationSignsNewVerifiesOld(t *testing.T) {
	ctx := context.Background()

	// Grant under the old keyset (k1 active).
	svcOld, _, _ := newTokenService(t)
	svcOld.WithKeys(keySetFor(t, "k1", map[string]string{"k1": rotOldKey}))
	_, grant, _, err := svcOld.PasswordGrant(ctx, "token@example.com", "password123", "ua", "1.2.3.4")
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if _, err := svcOld.VerifyAccess(grant.AccessToken); err != nil {
		t.Fatalf("verify before rotation: %v", err)
	}

	// Rotate: k2 becomes active while k1 stays listed.
	svcNew, _, _ := newTokenService(t)
	svcNew.WithKeys(keySetFor(t, "k2", map[string]string{"k1": rotOldKey, "k2": rotNewKey}))

	// The token signed under k1 still verifies (the rotation window).
	if _, err := svcNew.VerifyAccess(grant.AccessToken); err != nil {
		t.Fatalf("old-kid token must verify after rotation: %v", err)
	}

	// New grants carry the new kid and verify.
	_, grant2, _, err := svcNew.PasswordGrant(ctx, "token@example.com", "password123", "ua", "1.2.3.4")
	if err != nil {
		t.Fatalf("grant after rotation: %v", err)
	}
	parsed, _, err := new(jwt.Parser).ParseUnverified(grant2.AccessToken, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse new token: %v", err)
	}
	if parsed.Header["kid"] != "k2" {
		t.Fatalf("new token kid: want k2, got %v", parsed.Header["kid"])
	}
	if _, err := svcNew.VerifyAccess(grant2.AccessToken); err != nil {
		t.Fatalf("verify new token: %v", err)
	}
}

func TestAttackerTokenIsRejectedRegardlessOfKid(t *testing.T) {
	svc, _, _ := newTokenService(t)
	svc.WithKeys(keySetFor(t, "k1", map[string]string{"k1": rotOldKey}))

	// Signed by the attacker, claiming a known kid: signature verification
	// with the real k1 secret fails.
	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": tokens.Issuer, "sub": kernel.NewID(), "typ": "access",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(), "jti": "x",
	})
	forged.Header["kid"] = "k1"
	signed, err := forged.SignedString([]byte("attacker-key-that-is-long-enough"))
	if err != nil {
		t.Fatalf("forge: %v", err)
	}
	if _, err := svc.VerifyAccess(signed); err == nil {
		t.Fatal("a token signed by an attacker must not verify, kid or not")
	}
}

func TestLegacyTokensVerifyDuringMigration(t *testing.T) {
	ctx := context.Background()

	// Before the keyset: tokens have no kid and use APP_JWT_SECRET.
	svcOld, _, _ := newTokenService(t) // signs with testJWTSecret
	_, grant, _, err := svcOld.PasswordGrant(ctx, "token@example.com", "password123", "ua", "1.2.3.4")
	if err != nil {
		t.Fatalf("grant: %v", err)
	}

	// After the migration, with the old secret configured as legacy: the
	// kid-less token still verifies.
	svcNew, _, _ := newTokenService(t)
	svcNew.WithKeys(mustKeySet(t, keySetJSON(t, "k1", map[string]string{"k1": rotOldKey}), testJWTSecret))
	if _, err := svcNew.VerifyAccess(grant.AccessToken); err != nil {
		t.Fatalf("kid-less legacy token must verify while the legacy secret is configured: %v", err)
	}

	// Without a legacy secret, kid-less tokens are refused.
	svcNoLegacy, _, _ := newTokenService(t)
	svcNoLegacy.WithKeys(mustKeySet(t, keySetJSON(t, "k1", map[string]string{"k1": rotOldKey}), ""))
	if _, err := svcNoLegacy.VerifyAccess(grant.AccessToken); err == nil {
		t.Fatal("kid-less token without a configured legacy secret must be rejected")
	}
}
