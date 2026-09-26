// Package reliability holds small, dependency-free resilience helpers:
// bounded retry with exponential backoff and a bounded-concurrency runner.
package reliability

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"
)

// PermanentError marks an error that must not be retried; wrap a failure in
// it when retrying cannot change the outcome.
type PermanentError struct {
	Err error
}

func (e PermanentError) Error() string { return e.Err.Error() }
func (e PermanentError) Unwrap() error { return e.Err }

// Retry calls fn up to attempts times, sleeping between attempts with
// exponential backoff (capped) plus full jitter on top of baseDelay. It stops
// early when fn wraps a PermanentError or ctx is cancelled.
func Retry(ctx context.Context, attempts int, baseDelay time.Duration, fn func() error) error {
	if attempts <= 0 {
		attempts = 1
	}
	var last error
	for attempt := 1; attempt <= attempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		last = err
		var permanent PermanentError
		if errors.As(err, &permanent) || attempt == attempts {
			return err
		}
		jitter := time.Duration(rand.Int64N(int64(baseDelay)))
		delay := baseDelay*time.Duration(1<<min(attempt-1, 5)) + jitter
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return last
}

// RunBounded executes fn for indexes 0..total-1 using at most concurrency
// goroutines. It returns the first error collected, or ctx.Err when
// cancelled; cancellation stops dispatching and waits for in-flight calls.
func RunBounded(ctx context.Context, total, concurrency int, fn func(context.Context, int) error) error {
	if concurrency <= 0 {
		concurrency = 1
	}
	work := make(chan int)
	errs := make(chan error, total)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				if err := fn(ctx, i); err != nil {
					errs <- err
				}
			}
		}()
	}
	for i := range total {
		select {
		case <-ctx.Done():
			close(work)
			wg.Wait()
			return ctx.Err()
		case work <- i:
		}
	}
	close(work)
	wg.Wait()
	close(errs)
	for err := range errs {
		return err
	}
	return nil
}
