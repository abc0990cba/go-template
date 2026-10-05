package postgres

// The progressive-lockout counter, in PostgreSQL. A credential change
// (password set or reset through SetPasswordCredential) clears the guard:
// whoever legitimately proved control of the account starts fresh.

import (
	"context"
	"errors"
	"time"

	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/postgres/db"
)

var _ auth.LoginGuard = (*Store)(nil)

// notLocked is the store's "unlocked" deadline (the column's epoch default).
var notLocked = time.Unix(0, 0).UTC()

// GetLoginGuard returns the failure counter and lockout deadline; no row
// means zero failures and unlocked. An expired pin reads as unlocked.
func (s *Store) GetLoginGuard(ctx context.Context, userID string) (int, time.Time, error) {
	row, err := s.q.GetLoginGuard(ctx, userID)
	if err != nil {
		if errors.Is(mapErr(err), kernel.ErrNotFound) {
			return 0, time.Time{}, nil
		}
		return 0, time.Time{}, mapErr(err)
	}
	deadline := row.LockedUntil
	if deadline.Before(time.Now()) || deadline.Equal(notLocked) {
		deadline = time.Time{}
	}
	return int(row.FailedAttempts), deadline, nil
}

// RecordLoginFailure bumps the counter atomically and pins the deadline.
// The zero time pins the epoch, the store's "unlocked" value.
func (s *Store) RecordLoginFailure(ctx context.Context, userID string, _ time.Time, lockedUntil time.Time) (int, error) {
	if lockedUntil.IsZero() {
		lockedUntil = notLocked
	}
	attempts, err := s.q.RecordLoginFailure(ctx, db.RecordLoginFailureParams{
		UserID: userID, LockedUntil: lockedUntil,
	})
	return int(attempts), mapErr(err)
}

// ResetLoginGuard clears the counter and any lockout.
func (s *Store) ResetLoginGuard(ctx context.Context, userID string) error {
	_, err := s.q.ResetLoginGuard(ctx, userID)
	return mapErr(err)
}
