// Package revocations adapts Redis to the tokens.Revocations denylist: one
// key per revoked jti with a TTL equal to the token's remaining life, so
// entries clean themselves up and every replica sees the same list.
package revocations

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/your-org/go-template/internal/tokens"
)

const keyPrefix = "app:revoked_jti:"

// Redis is the shared denylist.
type Redis struct {
	client *redis.Client
}

var _ tokens.Revocations = (*Redis)(nil)

// New builds the Redis-backed denylist.
func New(client *redis.Client) *Redis { return &Redis{client: client} }

// RevokeJTI pins the jti until the given instant.
func (r *Redis) RevokeJTI(ctx context.Context, jti string, until time.Time) error {
	ttl := time.Until(until)
	if ttl <= 0 {
		return nil // already expired
	}
	return r.client.Set(ctx, keyPrefix+jti, "1", ttl).Err()
}

// IsRevokedJTI reports whether the jti is currently denylisted.
func (r *Redis) IsRevokedJTI(ctx context.Context, jti string) (bool, error) {
	n, err := r.client.Exists(ctx, keyPrefix+jti).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
