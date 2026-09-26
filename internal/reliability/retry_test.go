package reliability

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryStopsAfterSuccess(t *testing.T) {
	attempts := 0
	err := Retry(context.Background(), 3, time.Millisecond, func() error {
		attempts++
		if attempts == 2 {
			return nil
		}
		return errors.New("temporary")
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestRunBounded(t *testing.T) {
	err := RunBounded(context.Background(), 10, 3, func(_ context.Context, _ int) error { return nil })
	if err != nil {
		t.Fatalf("run bounded: %v", err)
	}
}
