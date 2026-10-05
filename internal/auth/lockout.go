package auth

// Progressive lockout: repeated password failures pin a lockout deadline on
// the account that doubles per failure past the threshold. The login answer
// never changes — unknown email, wrong password, and locked account all
// return ErrInvalidCredentials, so the gate cannot be used to probe which
// accounts exist or which ones an attacker already froze.

import (
	"context"
	"time"

	"github.com/your-org/go-template/internal/observ"
)

// MaxLockout caps the pinned deadline no matter how many failures stack, so
// an attacker cannot pin a permanent DoS on one account by grinding
// failures.
const MaxLockout = 24 * time.Hour

// zeroTime is the store's "not locked" value (timestamptz epoch 0).
var zeroTime = time.Unix(0, 0).UTC()

// LoginGuard persists the per-user failure counter and lockout deadline.
type LoginGuard interface {
	// GetLoginGuard returns the counter and the pinned lockout deadline
	// (zero time when unlocked); no rows yet means zero failures.
	GetLoginGuard(ctx context.Context, userID string) (int, time.Time, error)
	// RecordLoginFailure bumps the counter and pins the given deadline
	// (zero time leaves it unlocked); it returns the new count.
	RecordLoginFailure(ctx context.Context, userID string, now, lockedUntil time.Time) (int, error)
	// ResetLoginGuard clears the counter and any lockout.
	ResetLoginGuard(ctx context.Context, userID string) error
}

// WithLockout turns on progressive lockout: after threshold failures the
// account locks for window, doubling per additional failure up to MaxLockout.
// A threshold of 0 (or a nil guard) keeps the feature off.
func (s *Service) WithLockout(guard LoginGuard, threshold int, window time.Duration) *Service {
	if guard == nil || threshold <= 0 {
		return s
	}
	if window <= 0 {
		window = 15 * time.Minute
	}
	s.guard = guard
	s.lockoutThreshold = threshold
	s.lockoutWindow = window
	return s
}

// lockedOut reports whether the account is currently pinned.
func (s *Service) lockedOut(ctx context.Context, userID string) bool {
	if s.guard == nil {
		return false
	}
	_, lockedUntil, err := s.guard.GetLoginGuard(ctx, userID)
	if err != nil {
		// Fail open on a guard read problem: the rate limiters and the
		// password check still protect the account, and lockout must not
		// turn a store hiccup into an outage nobody can log in from.
		return false
	}
	return lockedUntil.After(s.now())
}

// recordFailure bumps the counter; past the threshold it pins the next
// lockout deadline and counts the transition in Prometheus.
func (s *Service) recordFailure(ctx context.Context, userID string) {
	if s.guard == nil {
		return
	}
	previous, _, _ := s.guard.GetLoginGuard(ctx, userID)
	attempts := previous + 1
	until := zeroTime
	if attempts >= s.lockoutThreshold {
		until = s.now().Add(s.lockoutFor(attempts))
		observ.AuthLockouts.Inc()
	}
	// A guard write problem must not mask the credential answer; the rate
	// limiters still front the endpoint.
	_, _ = s.guard.RecordLoginFailure(ctx, userID, s.now(), until)
}

// lockoutFor doubles the base window per failure past the threshold, capped
// at MaxLockout: attempt N past the threshold locks for window * 2^(N-threshold).
func (s *Service) lockoutFor(attempts int) time.Duration {
	excess := attempts - s.lockoutThreshold
	if excess > 16 {
		excess = 16 // keep the doubling bounded; MaxLockout caps anyway
	}
	window := s.lockoutWindow
	for range excess {
		window *= 2
		if window >= MaxLockout {
			return MaxLockout
		}
	}
	return window
}

// Unlock clears a locked account and its failure counter; the admin unlock
// endpoint lands here, and successful logins reset through it too.
func (s *Service) Unlock(ctx context.Context, userID string) error {
	if s.guard == nil {
		return nil
	}
	return s.guard.ResetLoginGuard(ctx, userID)
}
