// Package ratelimit provides the rate limiter used by the HTTP layer: a
// Redis sliding window for distributed deployments and an in-memory token
// bucket fallback for single-process development. Both satisfy one
// interface, so middleware does not know which one is behind it.
package ratelimit

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// Limiter admits or sheds one event per key. err reports an infrastructure
// failure; callers decide the fail-open/fail-closed policy.
type Limiter interface {
	Allow(ctx context.Context, key string) (ok bool, retryAfter time.Duration, err error)
}

// Memory is a per-process token bucket per key with idle-entry cleanup.
type Memory struct {
	mu      sync.Mutex
	rps     float64
	burst   int
	buckets map[string]*memoryBucket
}

type memoryBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewMemory builds the in-memory limiter.
func NewMemory(rps float64, burst int) *Memory {
	m := &Memory{rps: rps, burst: burst, buckets: map[string]*memoryBucket{}}
	go m.cleanup()
	return m
}

// Allow admits one event for the key.
func (m *Memory) Allow(_ context.Context, key string) (bool, time.Duration, error) {
	m.mu.Lock()
	b, ok := m.buckets[key]
	if !ok {
		b = &memoryBucket{limiter: rate.NewLimiter(rate.Limit(m.rps), m.burst)}
		m.buckets[key] = b
	}
	b.lastSeen = time.Now()
	limiter := b.limiter
	m.mu.Unlock()
	if limiter.Allow() {
		return true, 0, nil
	}
	return false, time.Second, nil
}

func (m *Memory) cleanup() {
	for {
		time.Sleep(time.Minute)
		m.mu.Lock()
		for key, b := range m.buckets {
			if time.Since(b.lastSeen) > 10*time.Minute {
				delete(m.buckets, key)
			}
		}
		m.mu.Unlock()
	}
}

// redisWindow implements a sliding-window log with a sorted set: precise
// enough for per-IP shedding, one round trip per decision.
const redisWindow = `
local removed = redis.call('ZREMRANGEBYSCORE', KEYS[1], 0, ARGV[1] - ARGV[2])
local count = redis.call('ZCARD', KEYS[1])
if count < tonumber(ARGV[3]) then
  redis.call('ZADD', KEYS[1], ARGV[1], ARGV[1] .. '-' .. ARGV[4])
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
  return {1, 0}
end
local oldest = redis.call('ZRANGE', KEYS[1], 0, 0, 'WITHSCORES')
redis.call('PEXPIRE', KEYS[1], ARGV[2])
return {0, oldest[2]}
`

// Redis is the distributed sliding-window limiter. One instance backs every
// api process, so the budget is shared cluster-wide.
type Redis struct {
	client *redis.Client
	script *redis.Script
	window time.Duration
	limit  int64
	member atomic.Uint64 // unique ZSET members without a random source
}

// NewRedis builds the distributed limiter over a window and request budget.
func NewRedis(client *redis.Client, window time.Duration, limit int) *Redis {
	return &Redis{client: client, script: redis.NewScript(redisWindow), window: window, limit: int64(limit)}
}

// Allow admits one event for the key.
func (r *Redis) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	nowMS := time.Now().UnixMilli()
	member := r.member.Add(1) // uniqueness only; ordering comes from the scores
	res, err := r.script.Run(ctx, r.client, []string{"ratelimit:" + key},
		strconv.FormatInt(nowMS, 10),
		strconv.FormatInt(r.window.Milliseconds(), 10),
		strconv.FormatInt(r.limit, 10),
		strconv.FormatUint(member, 10),
	).Slice()
	if err != nil {
		return false, 0, err
	}
	ok, _ := res[0].(int64)
	if ok == 1 {
		return true, 0, nil
	}
	// Res[1] is the oldest admission's score; retry when it leaves the window.
	oldestMS, _ := strconv.ParseInt(asString(res[1]), 10, 64)
	retryAfter := time.Duration(oldestMS+r.window.Milliseconds()-nowMS) * time.Millisecond
	if retryAfter < time.Millisecond {
		retryAfter = time.Millisecond
	}
	return false, retryAfter, nil
}

func asString(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	default:
		return "0"
	}
}
