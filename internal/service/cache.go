package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"golang.org/x/sync/singleflight"
)

// Cache is the read-through cache port for hot reads. Values are JSON
// encoded by the helpers; adapters only move bytes (memory in dev, Redis
// when configured — the same wire format works across replicas).
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration)
	Delete(ctx context.Context, keys ...string)
	DeletePrefix(ctx context.Context, prefix string)
}

// Cache keys live in one place so invalidation stays greppable.
const (
	CacheKeyUser  = "user:"  // + user id
	CacheKeyPerms = "perms:" // + domain + ":" + user id
	CacheTTLUser  = 30 * time.Second
	CacheTTLPerms = 30 * time.Second
)

// UserCacheKey is the canonical cache key for a user record.
func UserCacheKey(userID string) string { return CacheKeyUser + userID }

// PermsCacheKey is the canonical cache key for a user's expanded permission
// set inside one domain.
func PermsCacheKey(domain, userID string) string { return CacheKeyPerms + domain + ":" + userID }

// CachedGetOrLoad is the read-through helper: hit, else load under
// singleflight (a hot key miss must not stampede the store), else fill.
// Cache and loader failures degrade to the loader's error; the cache is
// best-effort by contract.
func CachedGetOrLoad[T any](ctx context.Context, cache Cache, logger *slog.Logger, group *singleflight.Group, key string, ttl time.Duration, load func(context.Context) (T, error)) (T, error) {
	var zero T
	if cache != nil {
		if raw, ok := cache.Get(ctx, key); ok {
			var value T
			if err := json.Unmarshal(raw, &value); err == nil {
				return value, nil
			} else if logger != nil {
				logger.Debug("cache decode failed", "key", key, "err", err)
			}
		}
	}
	result, err, _ := group.Do(key, func() (any, error) {
		value, err := load(ctx)
		if err != nil {
			return zero, err
		}
		if cache != nil {
			if raw, merr := json.Marshal(value); merr == nil {
				cache.Set(ctx, key, raw, ttl)
			}
		}
		return value, nil
	})
	if err != nil {
		return zero, err
	}
	value, ok := result.(T)
	if !ok {
		return zero, err
	}
	return value, nil
}

// CachedLoad mirrors CachedGetOrLoad for callers that need the cached flag
// (tests mostly).
func CachedLoad[T any](ctx context.Context, cache Cache, group *singleflight.Group, key string, ttl time.Duration, load func(context.Context) (T, error)) (T, bool, error) {
	if cache != nil {
		if raw, ok := cache.Get(ctx, key); ok {
			var value T
			if err := json.Unmarshal(raw, &value); err == nil {
				return value, true, nil
			}
		}
	}
	value, err := load(ctx)
	if err != nil {
		var zero T
		return zero, false, err
	}
	if cache != nil {
		if raw, merr := json.Marshal(value); merr == nil {
			cache.Set(ctx, key, raw, ttl)
		}
	}
	return value, false, nil
}
