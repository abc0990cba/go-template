// Memory-store refresh-token methods: JWT grant rotation families.

package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/tokens"
)

// --- Refresh tokens: JWT grant rotation families ---------------------------

var _ tokens.Store = (*MemoryStore)(nil)

// CreateRefreshToken stores a new refresh token.
func (s *MemoryStore) CreateRefreshToken(_ context.Context, token tokens.RefreshToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshTokens = append(s.refreshTokens, token)
	return nil
}

// GetRefreshTokenByHash resolves a refresh token by hash.
func (s *MemoryStore) GetRefreshTokenByHash(_ context.Context, tokenHash string) (tokens.RefreshToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, token := range s.refreshTokens {
		if token.TokenHash == tokenHash {
			return token, nil
		}
	}
	return tokens.RefreshToken{}, fmt.Errorf("%w: refresh token not found", kernel.ErrNotFound)
}

// RevokeRefreshToken marks one refresh token revoked.
func (s *MemoryStore) RevokeRefreshToken(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, token := range s.refreshTokens {
		if token.ID == id {
			if s.refreshTokens[i].RevokedAt == nil {
				now := time.Now()
				s.refreshTokens[i].RevokedAt = &now
			}
			return nil
		}
	}
	return fmt.Errorf("%w: refresh token not found", kernel.ErrNotFound)
}

// RevokeRefreshFamily revokes every token in a rotation family.
func (s *MemoryStore) RevokeRefreshFamily(_ context.Context, familyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i, token := range s.refreshTokens {
		if token.FamilyID == familyID && s.refreshTokens[i].RevokedAt == nil {
			s.refreshTokens[i].RevokedAt = &now
		}
	}
	return nil
}

// RevokeRefreshTokensForUser revokes every refresh token of a user.
func (s *MemoryStore) RevokeRefreshTokensForUser(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i, token := range s.refreshTokens {
		if token.UserID == userID && s.refreshTokens[i].RevokedAt == nil {
			s.refreshTokens[i].RevokedAt = &now
		}
	}
	return nil
}

// DeleteExpiredRefreshTokens purges expired refresh tokens.
func (s *MemoryStore) DeleteExpiredRefreshTokens(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	deadline := time.Now().Add(-7 * 24 * time.Hour)
	kept := s.refreshTokens[:0]
	for _, token := range s.refreshTokens {
		if token.ExpiresAt.After(deadline) {
			kept = append(kept, token)
		}
	}
	s.refreshTokens = kept
	return nil
}
