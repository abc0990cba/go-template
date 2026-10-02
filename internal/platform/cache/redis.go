package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis shares one cache across every replica: invalidation on one api
// process takes effect for all of them, which is the point of running this
// adapter in production.
type Redis struct {
	client *redis.Client
}

// NewRedis wraps an existing client (bootstrap owns the connection).
func NewRedis(client *redis.Client) *Redis {
	return &Redis{client: client}
}

// Get returns the cached value for the key, if present and fresh.
func (r *Redis) Get(ctx context.Context, key string) ([]byte, bool) {
	value, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	return value, true
}

// Set stores the value under the key until the TTL expires.
func (r *Redis) Set(ctx context.Context, key string, value []byte, ttl time.Duration) {
	_ = r.client.Set(ctx, key, value, ttl).Err()
}

// Delete drops the keys from the cache.
func (r *Redis) Delete(ctx context.Context, keys ...string) {
	if len(keys) == 0 {
		return
	}
	_ = r.client.Del(ctx, keys...).Err()
}

// DeletePrefix drops every key starting with the prefix.
func (r *Redis) DeletePrefix(ctx context.Context, prefix string) {
	iter := r.client.Scan(ctx, 0, prefix+"*", 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil || len(keys) == 0 {
		return
	}
	_ = r.client.Del(ctx, keys...).Err()
}
