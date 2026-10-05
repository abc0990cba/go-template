package ratelimit

import (
	"context"
	"testing"
)

func BenchmarkMemoryLimiter(b *testing.B) {
	limiter := NewMemory(1000, 1000)
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := limiter.Allow(ctx, "user-42"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMemoryLimiterDistinctKeys(b *testing.B) {
	limiter := NewMemory(1000, 1000)
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		key := "user-" + string(rune(b.N%64))
		if _, _, err := limiter.Allow(ctx, key); err != nil {
			b.Fatal(err)
		}
	}
}
