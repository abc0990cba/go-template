package repository

import (
	"context"
	"time"

	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/kernel"
)

// CreateEmailToken stores a hashed single-use token.
func (s *MemoryStore) CreateEmailToken(_ context.Context, token email.Token, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now()
	}
	s.emailTokens = append(s.emailTokens, emailTokenRow{token: token, hash: tokenHash})
	return nil
}

// GetEmailTokenByHash resolves a token by its hash.
func (s *MemoryStore) GetEmailTokenByHash(_ context.Context, tokenHash string) (email.Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, row := range s.emailTokens {
		if row.hash == tokenHash {
			return row.token, nil
		}
	}
	return email.Token{}, kernel.ErrNotFound
}

// MarkEmailTokenUsed consumes the token.
func (s *MemoryStore) MarkEmailTokenUsed(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, row := range s.emailTokens {
		if row.token.ID == id {
			if row.token.UsedAt != nil {
				return kernel.ErrNotFound
			}
			now := time.Now()
			s.emailTokens[i].token.UsedAt = &now
			return nil
		}
	}
	return kernel.ErrNotFound
}
