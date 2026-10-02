package cache_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/your-org/go-template/internal/cache"
	platformcache "github.com/your-org/go-template/internal/platform/cache"
)

func TestCachedGetOrLoadFillsAndHits(t *testing.T) {
	mem := platformcache.NewMemory()
	group := &singleflight.Group{}
	ctx := context.Background()
	calls := 0
	load := func(context.Context) (string, error) {
		calls++
		return "value", nil
	}

	for range 3 {
		value, err := cache.CachedGetOrLoad(ctx, mem, nil, group, "k", time.Minute, load)
		if err != nil || value != "value" {
			t.Fatalf("load: %v %v", value, err)
		}
	}
	if calls != 1 {
		t.Fatalf("loader must run once, ran %d times", calls)
	}

	// Loader errors are not cached.
	failing := func(context.Context) (string, error) { return "", errors.New("boom") }
	if _, err := cache.CachedGetOrLoad(ctx, mem, nil, group, "k2", time.Minute, failing); err == nil {
		t.Fatal("want loader error")
	}
	if _, ok := mem.Get(ctx, "k2"); ok {
		t.Fatal("errors must not be cached")
	}
}

func TestCachedGetOrLoadSingleflight(t *testing.T) {
	mem := platformcache.NewMemory()
	group := &singleflight.Group{}
	ctx := context.Background()
	var mu sync.Mutex
	calls := 0
	start := make(chan struct{})
	release := make(chan struct{})
	load := func(context.Context) (string, error) { //nolint:unparam // signature is fixed by CachedGetOrLoad; the failing-loader case is covered above
		mu.Lock()
		calls++
		first := calls == 1
		mu.Unlock()
		if first {
			close(start)
			<-release
		}
		return "shared", nil
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cache.CachedGetOrLoad(ctx, mem, nil, group, "hot", time.Minute, load)
		}()
	}
	<-start
	close(release)
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if calls > 2 {
		t.Fatalf("singleflight must collapse concurrent misses, loader ran %d times", calls)
	}
}
