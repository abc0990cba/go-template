package repository

import (
	"context"
	"time"

	"github.com/your-org/go-template/internal/apikeys"
	"github.com/your-org/go-template/internal/kernel"
)

// CreateAPIKey stores a key with its hash.
func (s *MemoryStore) CreateAPIKey(ctx context.Context, key apikeys.Key, keyHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key.CreatedAt.IsZero() {
		key.CreatedAt = time.Now()
	}
	s.apiKeys = append(s.apiKeys, apiKeyRow{key: key, hash: keyHash})
	return nil
}

// ListAPIKeysForUser returns the user's keys newest first.
func (s *MemoryStore) ListAPIKeysForUser(ctx context.Context, userID string) ([]apikeys.Key, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []apikeys.Key
	for _, row := range s.apiKeys {
		if row.key.UserID == userID {
			out = append(out, row.key)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// GetAPIKeyByPrefix resolves a live key by prefix and returns its hash.
func (s *MemoryStore) GetAPIKeyByPrefix(ctx context.Context, prefix string) (apikeys.Key, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, row := range s.apiKeys {
		if row.key.Prefix == prefix && row.key.RevokedAt == nil {
			return row.key, row.hash, nil
		}
	}
	return apikeys.Key{}, "", kernel.ErrNotFound
}

// TouchAPIKey stamps last use.
func (s *MemoryStore) TouchAPIKey(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, row := range s.apiKeys {
		if row.key.ID == id {
			now := time.Now()
			s.apiKeys[i].key.LastUsedAt = &now
		}
	}
	return nil
}

// RevokeAPIKey marks the key revoked.
func (s *MemoryStore) RevokeAPIKey(ctx context.Context, id, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, row := range s.apiKeys {
		if row.key.ID == id && row.key.UserID == userID {
			if row.key.RevokedAt != nil {
				return kernel.ErrNotFound
			}
			now := time.Now()
			s.apiKeys[i].key.RevokedAt = &now
			return nil
		}
	}
	return kernel.ErrNotFound
}
