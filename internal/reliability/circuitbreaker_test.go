package reliability

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBreakerOpensAfterConsecutiveFailures(t *testing.T) {
	b := NewBreaker("test", 3, 50*time.Millisecond)
	for i := range 2 {
		if err := b.Do(context.Background(), func() error { return errors.New("boom") }); err == nil {
			t.Fatalf("call %d: expected error", i)
		}
	}
	if b.State() != StateClosed {
		t.Fatalf("state after 2 failures = %v, want closed", b.State())
	}
	if err := b.Do(context.Background(), func() error { return errors.New("boom") }); err == nil {
		t.Fatal("third call: expected error")
	}
	if b.State() != StateOpen {
		t.Fatalf("state after 3 failures = %v, want open", b.State())
	}
	// While open, calls fail fast without running the function.
	called := false
	err := b.Do(context.Background(), func() error { called = true; return nil })
	if !errors.Is(err, ErrCircuitOpen) || called {
		t.Fatalf("open breaker: err=%v called=%v, want fast ErrCircuitOpen", err, called)
	}
}

func TestBreakerSuccessResetsFailureCount(t *testing.T) {
	b := NewBreaker("test", 3, 50*time.Millisecond)
	for range 2 {
		_ = b.Do(context.Background(), func() error { return errors.New("boom") })
	}
	if err := b.Do(context.Background(), func() error { return nil }); err != nil {
		t.Fatalf("good call: %v", err)
	}
	for range 2 {
		_ = b.Do(context.Background(), func() error { return errors.New("boom") })
	}
	if b.State() != StateClosed {
		t.Fatalf("state = %v, want closed (count should reset on success)", b.State())
	}
}

func TestBreakerHalfOpenProbe(t *testing.T) {
	b := NewBreaker("test", 1, 20*time.Millisecond)
	var transitions []string
	b.OnStateChange(func(_ string, from, to CircuitState) {
		transitions = append(transitions, from.String()+"->"+to.String())
	})

	_ = b.Do(context.Background(), func() error { return errors.New("boom") })
	if b.State() != StateOpen {
		t.Fatalf("state = %v, want open", b.State())
	}

	time.Sleep(25 * time.Millisecond)

	// One probe gets through; concurrent callers are rejected while it runs.
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	var probeErr error
	go func() {
		defer close(done)
		probeErr = b.Do(context.Background(), func() error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started
	if err := b.Allow(); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("second caller during probe: err=%v, want ErrCircuitOpen", err)
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("state during probe = %v, want half-open", b.State())
	}
	close(release)
	<-done
	if probeErr != nil {
		t.Fatalf("probe failed: %v", probeErr)
	}
	if b.State() != StateClosed {
		t.Fatalf("state after successful probe = %v, want closed", b.State())
	}

	want := []string{"closed->open", "open->half-open", "half-open->closed"}
	if len(transitions) != len(want) {
		t.Fatalf("transitions = %v, want %v", transitions, want)
	}
	for i := range want {
		if transitions[i] != want[i] {
			t.Fatalf("transitions = %v, want %v", transitions, want)
		}
	}
}

func TestBreakerFailedProbeReopens(t *testing.T) {
	b := NewBreaker("test", 1, 10*time.Millisecond)
	_ = b.Do(context.Background(), func() error { return errors.New("boom") })
	time.Sleep(15 * time.Millisecond)
	// The Do call itself is the half-open probe; its failure reopens.
	_ = b.Do(context.Background(), func() error { return errors.New("still down") })
	if b.State() != StateOpen {
		t.Fatalf("state after failed probe = %v, want open", b.State())
	}
	// And the fresh open period rejects calls again.
	if err := b.Allow(); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
}

func TestBreakerIgnoresContextCancellation(t *testing.T) {
	b := NewBreaker("test", 1, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := b.Do(ctx, func() error { return ctx.Err() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if b.State() != StateClosed {
		t.Fatalf("cancellation tripped the breaker: state = %v", b.State())
	}
}

func TestBreakersRegistryNilSafe(t *testing.T) {
	var nilRegistry *Breakers
	ran := false
	if err := nilRegistry.Do(context.Background(), "any", func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("nil registry must run fn directly: ran=%v err=%v", ran, err)
	}
	registry := NewBreakers(2, time.Second)
	a := registry.Get("a")
	b := registry.Get("a")
	if a != b {
		t.Fatal("Get must return the same breaker per name")
	}
	if len(registry.Items()) != 1 {
		t.Fatalf("Items = %d entries, want 1", len(registry.Items()))
	}
}

func TestBreakerConcurrentUse(t *testing.T) {
	b := NewBreaker("race", 5, time.Millisecond)
	var wg sync.WaitGroup
	var ok int64
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				_ = b.Do(context.Background(), func() error {
					if time.Now().UnixNano()%3 == 0 {
						return errors.New("boom")
					}
					return nil
				})
				atomic.AddInt64(&ok, 1)
			}
		}()
	}
	wg.Wait()
	if ok != 32*50 {
		t.Fatalf("completed %d calls, want %d", ok, 32*50)
	}
}
