// Memory-store MFA methods: TOTP factors, hashed recovery codes, and
// short-lived login challenges.

package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/mfa"
)

// --- MFA: TOTP factors, recovery codes, login challenges -------------------

// UpsertMFAFactor inserts or replaces the account's TOTP factor.
func (s *MemoryStore) UpsertMFAFactor(_ context.Context, factor mfa.Factor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mfaFactors[factor.UserID] = factor
	return nil
}

// ConfirmMFAFactor marks the factor confirmed.
func (s *MemoryStore) ConfirmMFAFactor(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	factor, ok := s.mfaFactors[userID]
	if !ok {
		return fmt.Errorf("%w: no pending factor", kernel.ErrNotFound)
	}
	now := time.Now()
	factor.Confirmed = true
	factor.ConfirmedAt = &now
	factor.UpdatedAt = now
	s.mfaFactors[userID] = factor
	return nil
}

// GetMFAFactor returns the account's TOTP factor.
func (s *MemoryStore) GetMFAFactor(_ context.Context, userID string) (mfa.Factor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	factor, ok := s.mfaFactors[userID]
	if !ok {
		return mfa.Factor{}, fmt.Errorf("%w: no factor", kernel.ErrNotFound)
	}
	return factor, nil
}

// DeleteMFAFactor removes the account's factor.
func (s *MemoryStore) DeleteMFAFactor(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.mfaFactors, userID)
	return nil
}

// CreateMFARecoveryCode appends one hashed recovery code.
func (s *MemoryStore) CreateMFARecoveryCode(_ context.Context, code mfa.RecoveryCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mfaCodes = append(s.mfaCodes, code)
	return nil
}

// ListMFARecoveryCodes returns the account's recovery codes.
func (s *MemoryStore) ListMFARecoveryCodes(_ context.Context, userID string) ([]mfa.RecoveryCode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []mfa.RecoveryCode
	for _, code := range s.mfaCodes {
		if code.UserID == userID {
			out = append(out, code)
		}
	}
	return out, nil
}

// ConsumeMFARecoveryCode marks one recovery code used.
func (s *MemoryStore) ConsumeMFARecoveryCode(_ context.Context, userID, codeHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, code := range s.mfaCodes {
		if code.UserID == userID && code.CodeHash == codeHash && code.UsedAt == nil {
			now := time.Now()
			s.mfaCodes[i].UsedAt = &now
			return nil
		}
	}
	return fmt.Errorf("%w: code not found or used", kernel.ErrNotFound)
}

// DeleteMFARecoveryCodes removes every recovery code of the account.
func (s *MemoryStore) DeleteMFARecoveryCodes(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	codes := s.mfaCodes[:0]
	for _, code := range s.mfaCodes {
		if code.UserID != userID {
			codes = append(codes, code)
		}
	}
	s.mfaCodes = codes
	return nil
}

// CreateMFAChallenge stores a pending login challenge.
func (s *MemoryStore) CreateMFAChallenge(_ context.Context, challenge mfa.Challenge) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mfaChallenges = append(s.mfaChallenges, challenge)
	return nil
}

// GetMFAChallengeByHash resolves a challenge by code hash.
func (s *MemoryStore) GetMFAChallengeByHash(_ context.Context, tokenHash string) (mfa.Challenge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, challenge := range s.mfaChallenges {
		if challenge.TokenHash == tokenHash {
			return challenge, nil
		}
	}
	return mfa.Challenge{}, fmt.Errorf("%w: challenge not found", kernel.ErrNotFound)
}

// IncrementMFAChallengeAttempts bumps the challenge's attempt counter.
func (s *MemoryStore) IncrementMFAChallengeAttempts(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, challenge := range s.mfaChallenges {
		if challenge.ID == id {
			s.mfaChallenges[i].Attempts++
			return nil
		}
	}
	return fmt.Errorf("%w: challenge not found", kernel.ErrNotFound)
}

// DeleteMFAChallenge removes the challenge.
func (s *MemoryStore) DeleteMFAChallenge(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	challenges := s.mfaChallenges[:0]
	for _, challenge := range s.mfaChallenges {
		if challenge.ID != id {
			challenges = append(challenges, challenge)
		}
	}
	s.mfaChallenges = challenges
	return nil
}

// DeleteExpiredMFAChallenges purges challenges past their expiry.
func (s *MemoryStore) DeleteExpiredMFAChallenges(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	challenges := s.mfaChallenges[:0]
	for _, challenge := range s.mfaChallenges {
		if challenge.ExpiresAt.After(now) {
			challenges = append(challenges, challenge)
		}
	}
	s.mfaChallenges = challenges
	return nil
}
