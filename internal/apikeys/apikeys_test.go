// External test package: the memory store (repository) implements
// apikeys.Store, so a same-package test importing it would cycle.
package apikeys_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/apikeys"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/repository"
)

func newService(t *testing.T) *apikeys.Service {
	t.Helper()
	return apikeys.NewService(repository.NewMemoryStore())
}

func TestScopesAllow(t *testing.T) {
	cases := []struct {
		name   string
		scopes []string
		obj    string
		act    string
		want   bool
	}{
		{"exact match", []string{"users:read"}, "users", "read", true},
		{"object wildcard", []string{"users:*"}, "users", "read", true},
		{"object wildcard other action", []string{"users:*"}, "users", "delete", true},
		{"action mismatch", []string{"users:read"}, "users", "write", false},
		{"object mismatch", []string{"tickets:*"}, "users", "read", false},
		{"second scope matches", []string{"users:read", "jobs:execute"}, "jobs", "execute", true},
		{"no scopes at all", nil, "users", "read", false},
		{"empty scopes", []string{}, "users", "read", false},
		{"no global wildcard", []string{"*"}, "users", "read", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := apikeys.ScopesAllow(tc.scopes, tc.obj, tc.act); got != tc.want {
				t.Errorf("ScopesAllow(%v, %s, %s): want %v, got %v", tc.scopes, tc.obj, tc.act, tc.want, got)
			}
		})
	}
}

func TestCreateAndResolve(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	// A name is required.
	if _, _, err := svc.Create(ctx, "u1", "", nil, 0); !kernel.IsValidationError(err) {
		t.Fatalf("create without name: want validation error, got %v", err)
	}

	key, secret, err := svc.Create(ctx, "u1", "ci", []string{"users:read"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(secret) <= len(apikeys.Prefix) || secret[:len(apikeys.Prefix)] != apikeys.Prefix {
		t.Fatalf("secret: want gtpl_…, got %q", secret)
	}
	if key.Prefix != secret[:len(apikeys.Prefix)+6] {
		t.Errorf("stored prefix: want first %d chars of secret, got %q", len(apikeys.Prefix)+6, key.Prefix)
	}
	if key.ExpiresAt == nil || !key.ExpiresAt.After(time.Now().Add(23*time.Hour)) {
		t.Errorf("expires at: want ~24h out, got %v", key.ExpiresAt)
	}

	// The raw secret resolves to its owner...
	resolved, owner, err := svc.Resolve(ctx, secret)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.ID != key.ID || owner.ID != "u1" {
		t.Errorf("resolve: want key %s/owner u1, got %s/%s", key.ID, resolved.ID, owner.ID)
	}

	// ...and stamps last use on the stored record.
	listed, err := svc.List(ctx, "u1")
	if err != nil || len(listed) != 1 {
		t.Fatalf("list: %d keys (%v)", len(listed), err)
	}
	if listed[0].LastUsedAt == nil {
		t.Error("resolve: want LastUsedAt stamped on the stored key")
	}

	// Wrong and foreign-shaped tokens are indistinguishable misses.
	for _, bad := range []string{apikeys.Prefix + "wrong-value-here", "session-token", ""} {
		if _, _, err := svc.Resolve(ctx, bad); !errors.Is(err, kernel.ErrNotFound) {
			t.Errorf("resolve %q: want ErrNotFound, got %v", bad, err)
		}
	}
}

func TestCreateWithoutTTLNeverExpires(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	key, secret, err := svc.Create(ctx, "u1", "forever", []string{"*:*"}, 0)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if key.ExpiresAt != nil {
		t.Errorf("no-ttl key: want nil expiry, got %v", key.ExpiresAt)
	}
	if _, _, err := svc.Resolve(ctx, secret); err != nil {
		t.Errorf("resolve no-ttl key: %v", err)
	}
}

func TestExpiredKeyDoesNotResolve(t *testing.T) {
	ctx := context.Background()
	// The clock is walked past the TTL instead of seeding an already-dead
	// row: resolution is where expiry is enforced.
	current := time.Unix(1700000000, 0).UTC()
	store := repository.NewMemoryStore()
	svc := apikeys.NewService(store).WithClock(func() time.Time { return current })

	_, secret, err := svc.Create(ctx, "u1", "ci", nil, time.Hour)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := svc.Resolve(ctx, secret); err != nil {
		t.Fatalf("live key: %v", err)
	}

	current = current.Add(2 * time.Hour)
	if _, _, err := svc.Resolve(ctx, secret); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("expired key resolve: want ErrNotFound, got %v", err)
	}
}

func TestRevoke(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	key, secret, err := svc.Create(ctx, "u1", "ci", nil, 0)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Another user cannot revoke the key.
	if err := svc.Revoke(ctx, key.ID, "u2"); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("revoke by stranger: want ErrNotFound, got %v", err)
	}

	if err := svc.Revoke(ctx, key.ID, "u1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	// A revoked key stops resolving immediately...
	if _, _, err := svc.Resolve(ctx, secret); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("revoked key resolve: want ErrNotFound, got %v", err)
	}
	// ...and double revocation is a miss, not a success.
	if err := svc.Revoke(ctx, key.ID, "u1"); !errors.Is(err, kernel.ErrNotFound) {
		t.Errorf("double revoke: want ErrNotFound, got %v", err)
	}
}
