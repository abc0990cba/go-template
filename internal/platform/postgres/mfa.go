package postgres

import (
	"context"

	"github.com/your-org/go-template/internal/mfa"
	"github.com/your-org/go-template/internal/platform/postgres/db"
)

// The PostgreSQL adapter implements the mfa feature's store port.
var _ mfa.Store = (*Store)(nil)

// UpsertMFAFactor inserts or replaces the account's TOTP factor.
func (s *Store) UpsertMFAFactor(ctx context.Context, factor mfa.Factor) error {
	return mapErr(s.q.UpsertMFAFactor(ctx, db.UpsertMFAFactorParams{
		UserID: factor.UserID,
		Secret: factor.Secret,
	}))
}

// ConfirmMFAFactor marks the factor confirmed.
func (s *Store) ConfirmMFAFactor(ctx context.Context, userID string) error {
	return mapErr(s.q.ConfirmMFAFactor(ctx, userID))
}

// GetMFAFactor returns the account's TOTP factor.
func (s *Store) GetMFAFactor(ctx context.Context, userID string) (mfa.Factor, error) {
	row, err := s.q.GetMFAFactor(ctx, userID)
	if err != nil {
		return mfa.Factor{}, mapErr(err)
	}
	out := mfa.Factor{
		UserID:    row.UserID,
		Secret:    row.Secret,
		Confirmed: row.Confirmed,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	if row.ConfirmedAt.Valid {
		t := row.ConfirmedAt.Time
		out.ConfirmedAt = &t
	}
	return out, nil
}

// DeleteMFAFactor removes the account's factor.
func (s *Store) DeleteMFAFactor(ctx context.Context, userID string) error {
	return mapErr(s.q.DeleteMFAFactor(ctx, userID))
}

// CreateMFARecoveryCode appends one hashed recovery code.
func (s *Store) CreateMFARecoveryCode(ctx context.Context, code mfa.RecoveryCode) error {
	return mapErr(s.q.CreateMFARecoveryCode(ctx, db.CreateMFARecoveryCodeParams{
		ID:       code.ID,
		UserID:   code.UserID,
		CodeHash: code.CodeHash,
	}))
}

// ListMFARecoveryCodes returns the account's recovery codes.
func (s *Store) ListMFARecoveryCodes(ctx context.Context, userID string) ([]mfa.RecoveryCode, error) {
	rows, err := s.q.ListMFARecoveryCodes(ctx, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]mfa.RecoveryCode, 0, len(rows))
	for _, row := range rows {
		out = append(out, mfaRecoveryCode(row))
	}
	return out, nil
}

// ConsumeMFARecoveryCode marks one recovery code used.
func (s *Store) ConsumeMFARecoveryCode(ctx context.Context, userID, codeHash string) error {
	code, err := s.q.GetUnusedMFARecoveryCodeByHash(ctx, db.GetUnusedMFARecoveryCodeByHashParams{
		UserID: userID, CodeHash: codeHash,
	})
	if err != nil {
		return mapErr(err)
	}
	return mapErr(s.q.MarkMFARecoveryCodeUsed(ctx, code.ID))
}

// DeleteMFARecoveryCodes removes every recovery code of the account.
func (s *Store) DeleteMFARecoveryCodes(ctx context.Context, userID string) error {
	return mapErr(s.q.DeleteMFARecoveryCodes(ctx, userID))
}

// CreateMFAChallenge stores a pending login challenge.
func (s *Store) CreateMFAChallenge(ctx context.Context, challenge mfa.Challenge) error {
	return mapErr(s.q.CreateMFAChallenge(ctx, db.CreateMFAChallengeParams{
		ID:        challenge.ID,
		UserID:    challenge.UserID,
		TokenHash: challenge.TokenHash,
		ExpiresAt: challenge.ExpiresAt,
	}))
}

// GetMFAChallengeByHash resolves a challenge by code hash.
func (s *Store) GetMFAChallengeByHash(ctx context.Context, tokenHash string) (mfa.Challenge, error) {
	row, err := s.q.GetMFAChallengeByHash(ctx, tokenHash)
	if err != nil {
		return mfa.Challenge{}, mapErr(err)
	}
	return mfa.Challenge{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		Attempts:  int(row.Attempts),
		ExpiresAt: row.ExpiresAt,
		CreatedAt: row.CreatedAt,
	}, nil
}

// IncrementMFAChallengeAttempts bumps the challenge's attempt counter.
func (s *Store) IncrementMFAChallengeAttempts(ctx context.Context, id string) error {
	return mapErr(s.q.IncrementMFAChallengeAttempts(ctx, id))
}

// DeleteMFAChallenge removes the challenge.
func (s *Store) DeleteMFAChallenge(ctx context.Context, id string) error {
	return mapErr(s.q.DeleteMFAChallenge(ctx, id))
}

// DeleteExpiredMFAChallenges purges challenges past their expiry.
func (s *Store) DeleteExpiredMFAChallenges(ctx context.Context) error {
	return mapErr(s.q.DeleteExpiredMFAChallenges(ctx))
}

func mfaRecoveryCode(row db.MfaRecoveryCode) mfa.RecoveryCode {
	out := mfa.RecoveryCode{
		ID:        row.ID,
		UserID:    row.UserID,
		CodeHash:  row.CodeHash,
		CreatedAt: row.CreatedAt,
	}
	if row.UsedAt.Valid {
		t := row.UsedAt.Time
		out.UsedAt = &t
	}
	return out
}
