package auth_test

// Progressive lockout: threshold, doubling, reset on success, expiry by the
// injected clock, and the uniform ErrInvalidCredentials answer.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/repository"
)

// memGuard is an in-memory LoginGuard for policy tests.
type memGuard struct {
	attempts map[string]int
	locked   map[string]time.Time
}

func newMemGuard() *memGuard {
	return &memGuard{attempts: map[string]int{}, locked: map[string]time.Time{}}
}

func (g *memGuard) GetLoginGuard(_ context.Context, userID string) (int, time.Time, error) {
	return g.attempts[userID], g.locked[userID], nil
}

func (g *memGuard) RecordLoginFailure(_ context.Context, userID string, _ time.Time, until time.Time) (int, error) {
	g.attempts[userID]++
	g.locked[userID] = until
	return g.attempts[userID], nil
}

func (g *memGuard) ResetLoginGuard(_ context.Context, userID string) error {
	delete(g.attempts, userID)
	delete(g.locked, userID)
	return nil
}

type lockoutFixture struct {
	svc   *auth.Service
	guard *memGuard
	now   time.Time
	uid   string
}

func newLockoutFixture(t *testing.T, threshold int) *lockoutFixture {
	t.Helper()
	store := repository.NewMemoryStore()
	f := &lockoutFixture{
		svc:   auth.NewService(store, nil, time.Hour),
		guard: newMemGuard(),
		now:   time.Unix(1_800_000_000, 0),
	}
	f.svc.WithClock(func() time.Time { return f.now }).WithLockout(f.guard, threshold, 15*time.Minute)
	user, _, err := f.svc.Register(t.Context(), "locked@example.com", "Ada", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	f.uid = user.ID
	return f
}

func TestLockoutAcceptsHealthyLoginsUntouched(t *testing.T) {
	f := newLockoutFixture(t, 5)
	if _, _, err := f.svc.Login(t.Context(), "locked@example.com", "password123"); err != nil {
		t.Fatalf("correct password: %v", err)
	}
	if len(f.guard.attempts) != 0 {
		t.Fatalf("successful login must clear the guard, got %v", f.guard.attempts)
	}
}

func TestLockoutPinsAfterThresholdAndExpires(t *testing.T) {
	f := newLockoutFixture(t, 3)
	ctx := t.Context()

	// Failures 1 and 2 bump the counter without locking.
	for range 2 {
		if _, _, err := f.svc.Login(ctx, "locked@example.com", "wrong"); !errors.Is(err, kernel.ErrInvalidCredentials) {
			t.Fatalf("wrong password: want invalid credentials, got %v", err)
		}
	}
	if _, until, _ := f.guard.GetLoginGuard(ctx, f.uid); until.After(f.now) {
		t.Fatalf("no lock before the threshold, got %v", until)
	}

	// Failure 3 pins the base 15-minute window.
	_, _, _ = f.svc.Login(ctx, "locked@example.com", "wrong")
	_, until, _ := f.guard.GetLoginGuard(ctx, f.uid)
	if want := f.now.Add(15 * time.Minute); !until.Equal(want) {
		t.Fatalf("first lock: want %v, got %v", want, until)
	}

	// While locked, the correct password still answers invalid credentials
	// — the gate must not tell the attacker they already own the password.
	if _, _, err := f.svc.Login(ctx, "locked@example.com", "password123"); !errors.Is(err, kernel.ErrInvalidCredentials) {
		t.Fatalf("locked login: want invalid credentials, got %v", err)
	}

	// After expiry the correct password works again and the counter resets.
	f.now = until.Add(time.Second)
	if _, _, err := f.svc.Login(ctx, "locked@example.com", "password123"); err != nil {
		t.Fatalf("post-lock login: %v", err)
	}
	if len(f.guard.attempts) != 0 {
		t.Fatalf("success must clear the guard: %v", f.guard.attempts)
	}
}

func TestLockoutDoublingIsCapped(t *testing.T) {
	f := newLockoutFixture(t, 2)
	ctx := t.Context()
	// Grind failures; from the threshold on, every pinned window doubles but
	// must stay within the cap.
	var lastLock time.Time
	for attempt := 1; attempt <= 30; attempt++ {
		f.now = lastLock.Add(time.Second) // expired pin reads as unlocked
		_, _, _ = f.svc.Login(ctx, "locked@example.com", "wrong")
		_, until, _ := f.guard.GetLoginGuard(ctx, f.uid)
		if attempt >= 2 && !until.IsZero() && until.Sub(f.now) > auth.MaxLockout {
			t.Fatalf("attempt %d: window %v exceeds the cap", attempt, until.Sub(f.now))
		}
		lastLock = until
	}
}

func TestLockoutZeroThresholdDisables(t *testing.T) {
	f := newLockoutFixture(t, 0)
	ctx := t.Context()
	// Grind failures: nothing may ever lock.
	for range 20 {
		_, _, _ = f.svc.Login(ctx, "locked@example.com", "wrong")
	}
	if _, _, err := f.svc.Login(ctx, "locked@example.com", "password123"); err != nil {
		t.Fatalf("disabled lockout must never lock: %v", err)
	}
	if len(f.guard.attempts) != 0 {
		t.Fatalf("disabled lockout must not track attempts: %v", f.guard.attempts)
	}
}

func TestUnknownEmailDoesNotTouchTheGuard(t *testing.T) {
	f := newLockoutFixture(t, 1)
	for range 5 {
		_, _, _ = f.svc.Login(t.Context(), "ghost@example.com", "wrong")
	}
	if len(f.guard.attempts) != 0 {
		t.Fatalf("unknown emails must not create guard rows: %v", f.guard.attempts)
	}
}

func TestUnlockClearsTheGuard(t *testing.T) {
	f := newLockoutFixture(t, 1)
	ctx := t.Context()
	_, _, _ = f.svc.Login(ctx, "locked@example.com", "wrong") // locks immediately
	if _, _, err := f.svc.Login(ctx, "locked@example.com", "password123"); !errors.Is(err, kernel.ErrInvalidCredentials) {
		t.Fatalf("locked login: want invalid credentials, got %v", err)
	}

	// The admin unlock endpoint's service call: the account is immediately
	// usable again, without waiting for the window.
	if err := f.svc.Unlock(ctx, f.uid); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if _, _, err := f.svc.Login(ctx, "locked@example.com", "password123"); err != nil {
		t.Fatalf("post-unlock login: %v", err)
	}
}

func TestPwnedGateRejectsBreachedPassword(t *testing.T) {
	store := repository.NewMemoryStore()
	svc := auth.NewService(store, nil, time.Hour).WithPwned(func(_ context.Context, password string) (bool, error) {
		return password == "breached123", nil
	})
	_, _, err := svc.Register(t.Context(), "pwned@example.com", "Ada", "breached123")
	if !errors.Is(err, kernel.Invalid("")) && err == nil {
		t.Fatalf("breached password: want validation error, got %v", err)
	}
	if _, _, err := svc.Register(t.Context(), "pwned@example.com", "Ada", "freshpassword1"); err != nil {
		t.Fatalf("fresh password: %v", err)
	}
	// A checker failure must not block registration (fail-open policy
	// lives in the wrapper, and errors are never treated as compromises).
	_, _, err = svc.Register(t.Context(), "pwned2@example.com", "Ada", "anotherpass1")
	if err != nil {
		t.Fatalf("checker error must not reject: %v", err)
	}
}
