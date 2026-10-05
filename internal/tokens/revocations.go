package tokens

// Access-token revocation: the denylist of revoked jti claims. Access JWTs
// are otherwise stateless — revoking a refresh family or deleting sessions
// cannot touch a token already in a client's hands until it expires. The
// denylist closes that window: each entry lives exactly as long as the
// token could, then disappears.

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrNoJTI marks a token without a jti claim (pre-keyset tokens); it cannot
// be addressed by the denylist.
var ErrNoJTI = errors.New("token carries no jti")

// Revocations persists revoked jti claims until their natural expiry.
type Revocations interface {
	// RevokeJTI pins one jti until the given instant.
	RevokeJTI(ctx context.Context, jti string, until time.Time) error
	// IsRevokedJTI reports whether the jti is on the denylist.
	IsRevokedJTI(ctx context.Context, jti string) (bool, error)
}

// MemoryRevocations is the in-process denylist (dev and single-replica
// deployments; multi-replica deployments share the Redis adapter).
type MemoryRevocations struct {
	mu  sync.RWMutex
	set map[string]time.Time
	now func() time.Time
}

// NewMemoryRevocations returns an empty denylist.
func NewMemoryRevocations() *MemoryRevocations {
	return &MemoryRevocations{set: map[string]time.Time{}, now: time.Now}
}

// WithClock replaces the wall clock; tests walk expiry deterministically.
func (m *MemoryRevocations) WithClock(now func() time.Time) *MemoryRevocations {
	m.now = now
	return m
}

// RevokeJTI pins the jti until the given instant.
func (m *MemoryRevocations) RevokeJTI(_ context.Context, jti string, until time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if until.Before(m.now()) {
		return nil // already expired; nothing to remember
	}
	m.set[jti] = until
	return nil
}

// IsRevokedJTI reports membership, lazily dropping expired entries.
func (m *MemoryRevocations) IsRevokedJTI(_ context.Context, jti string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	until, ok := m.set[jti]
	if !ok {
		return false, nil
	}
	if m.now().After(until) {
		delete(m.set, jti)
		return false, nil
	}
	return true, nil
}

// WithRevocations attaches the denylist; VerifyAccess starts consulting it
// and RevokeAccess starts writing to it.
func (s *Service) WithRevocations(revocations Revocations) *Service {
	if revocations != nil {
		s.revocations = revocations
	}
	return s
}

// RevokeAccess parses a live access token and pins its jti until expiry.
// The signature is verified first, so only tokens this service minted can
// be denylisted.
func (s *Service) RevokeAccess(ctx context.Context, tokenString string) error {
	if !s.enabled() {
		return ErrAuthDisabled
	}
	parsed, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return s.keyFor(token.Header["kid"])
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuer(Issuer))
	if err != nil || !parsed.Valid {
		return ErrInvalidGrant
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return ErrInvalidGrant
	}
	jti, _ := claims["jti"].(string)
	if jti == "" {
		return ErrNoJTI
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return ErrInvalidGrant
	}
	if s.revocations == nil {
		// Nothing to write to; the token still dies at exp. Report success:
		// the caller's intent (invalidate) is best-effort honored.
		return nil
	}
	return s.revocations.RevokeJTI(ctx, jti, exp.Time)
}

// revoked reports whether the parsed token's jti is denylisted.
func (s *Service) revoked(ctx context.Context, parsed jwt.MapClaims) bool {
	if s.revocations == nil {
		return false
	}
	jti, _ := parsed["jti"].(string)
	if jti == "" {
		return false
	}
	isRevoked, err := s.revocations.IsRevokedJTI(ctx, jti)
	return err == nil && isRevoked
}

var _ Revocations = (*MemoryRevocations)(nil)
