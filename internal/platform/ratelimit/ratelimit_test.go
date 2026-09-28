package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryAllowsBurstThenSheds(t *testing.T) {
	limiter := NewMemory(5, 2)
	ctx := context.Background()
	admitted := 0
	for range 5 {
		ok, _, err := limiter.Allow(ctx, "ip-1")
		if err != nil {
			t.Fatalf("allow: %v", err)
		}
		if ok {
			admitted++
		}
	}
	if admitted != 2 {
		t.Fatalf("admitted %d requests, want the burst of 2", admitted)
	}
	// Different keys are independent.
	if ok, _, _ := limiter.Allow(ctx, "ip-2"); !ok {
		t.Fatal("second key should have its own budget")
	}
}

func TestMemoryConcurrentUse(_ *testing.T) {
	limiter := NewMemory(1000, 1000)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				_, _, _ = limiter.Allow(context.Background(), "shared")
			}
		}()
	}
	wg.Wait()
}

func TestRedisWindowScriptShape(_ *testing.T) {
	// The script is compiled against the redis server in the integration
	// suite; here we only pin the argument contract.
	if NewRedis(nil, time.Second, 10).window != time.Second {
		panic("window not stored")
	}
}
