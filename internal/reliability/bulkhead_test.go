package reliability

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBulkheadLimitsConcurrency(t *testing.T) {
	const limit = 3
	b := NewBulkhead("test.http", limit, 50*time.Millisecond)

	var inFlight, peak atomic.Int64
	var wg sync.WaitGroup
	for range limit * 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := b.Do(context.Background(), func() error {
				cur := inFlight.Add(1)
				for {
					p := peak.Load()
					if cur <= p || peak.CompareAndSwap(p, cur) {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
				inFlight.Add(-1)
				return nil
			})
			if err != nil {
				t.Errorf("call rejected unexpectedly: %v", err)
			}
		}()
	}
	wg.Wait()
	if peak.Load() > limit {
		t.Fatalf("peak in-flight %d exceeded bulkhead limit %d", peak.Load(), limit)
	}
}

func TestBulkheadRejectsWhenFull(t *testing.T) {
	b := NewBulkhead("slow.http", 1, 30*time.Millisecond)

	release := make(chan struct{})
	unblocked := make(chan struct{})
	go func() {
		_ = b.Do(context.Background(), func() error {
			close(unblocked)
			<-release
			return nil
		})
	}()
	<-unblocked

	err := b.Do(context.Background(), func() error { return nil })
	if err == nil {
		t.Fatal("expected ErrBulkheadFull while the single slot is held")
	}
	var full errBulkheadFull
	if !asBulkheadFull(err, &full) {
		t.Fatalf("expected ErrBulkheadFull, got %v", err)
	}
	close(release)
}

func TestBulkheadHonorsContext(t *testing.T) {
	b := NewBulkhead("ctx.http", 1, time.Second) // generous acquire timeout

	release := make(chan struct{})
	acquired := make(chan struct{})
	go func() {
		_ = b.Do(context.Background(), func() error {
			close(acquired) // the slot is ours; the caller below cannot win
			<-release
			return nil
		})
	}()
	<-acquired

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := b.Do(ctx, func() error { return nil }); err != context.DeadlineExceeded {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	close(release)
}

func TestNilBulkheadsRunsDirectly(t *testing.T) {
	var r *Bulkheads
	called := false
	if err := r.Do(context.Background(), "any.http", func() error { called = true; return nil }); err != nil {
		t.Fatalf("nil registry returned error: %v", err)
	}
	if !called {
		t.Fatal("nil registry did not run the call")
	}
}

func TestBulkheadsObserversFire(t *testing.T) {
	r := NewBulkheads(1, 50*time.Millisecond)
	var inFlightDeltas int
	var rejected int
	r.SetObservers(func(_ string, delta int) { inFlightDeltas += delta },
		func(_ string) { rejected++ })

	_ = r.Get("obs.http").Do(context.Background(), func() error { return nil })
	if inFlightDeltas != 0 {
		t.Fatalf("in-flight deltas did not balance: %d", inFlightDeltas)
	}

	release := make(chan struct{})
	acquired := make(chan struct{})
	go func() {
		_ = r.Get("obs.http").Do(context.Background(), func() error {
			close(acquired)
			<-release
			return nil
		})
	}()
	<-acquired // the slot is held; the next caller must be rejected
	_ = r.Get("obs.http").Do(context.Background(), func() error { return nil })
	close(release)

	if rejected != 1 {
		t.Fatalf("expected exactly one rejection, got %d", rejected)
	}
}

func asBulkheadFull(err error, target *errBulkheadFull) bool {
	full, ok := err.(errBulkheadFull)
	if ok {
		*target = full
	}
	return ok
}
