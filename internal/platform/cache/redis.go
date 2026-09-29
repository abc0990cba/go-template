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

func (r *Redis) Get(ctx context.Context, key string) ([]byte, bool) {
	value, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	return value, true
}

func (r *Redis) Set(ctx context.Context, key string, value []byte, ttl time.Duration) {
	_ = r.client.Set(ctx, key, value, ttl).Err()
}

func (r *Redis) Delete(ctx context.Context, keys ...string) {
	if len(keys) == 0 {
		return
	}
	_ = r.client.Del(ctx, keys...).Err()
}

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
