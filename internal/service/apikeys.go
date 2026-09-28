package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
)

// APIKeyPrefix marks personal API keys on the wire so the auth middleware
// can route them to this resolver instead of session lookup.
const APIKeyPrefix = "gtpl_"

// APIKeysRepo persists personal API keys. Hashes never leave the store.
type APIKeysRepo interface {
	CreateAPIKey(ctx context.Context, key model.APIKey, keyHash string) error
	ListAPIKeysForUser(ctx context.Context, userID string) ([]model.APIKey, error)
	GetAPIKeyByPrefix(ctx context.Context, prefix string) (key model.APIKey, keyHash string, err error)
	TouchAPIKey(ctx context.Context, id string) error
	RevokeAPIKey(ctx context.Context, id, userID string) error
}

// APIKeysService issues and resolves personal API keys. The raw key is
// shown exactly once at creation; only its hash is stored.
type APIKeysService struct {
	store APIKeysRepo
}

// NewAPIKeysService builds the service.
func NewAPIKeysService(store APIKeysRepo) *APIKeysService { return &APIKeysService{store: store} }

// Create issues a key. Scopes are "obj:act" strings that must be a subset
// of the owner's own permissions — the key can never widen access.
func (s *APIKeysService) Create(ctx context.Context, userID, name string, scopes []string, ttl time.Duration) (model.APIKey, string, error) {
	if name == "" {
		return model.APIKey{}, "", validationError("key name is required")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return model.APIKey{}, "", err
	}
	secret := APIKeyPrefix + base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(secret))
	now := time.Now()
	var expires *time.Time
	if ttl > 0 {
		t := now.Add(ttl)
		expires = &t
	}
	key := model.APIKey{
		ID: newID(), UserID: userID, Name: name,
		Prefix:    secret[:len(APIKeyPrefix)+6],
		Scopes:    scopes,
		CreatedAt: now, ExpiresAt: expires,
	}
	if err := s.store.CreateAPIKey(ctx, key, encodeHash(sum)); err != nil {
		return model.APIKey{}, "", err
	}
	return key, secret, nil
}

// List returns the caller's keys, newest first.
func (s *APIKeysService) List(ctx context.Context, userID string) ([]model.APIKey, error) {
	return s.store.ListAPIKeysForUser(ctx, userID)
}

// Revoke marks the key revoked; it disappears from resolution immediately.
func (s *APIKeysService) Revoke(ctx context.Context, id, userID string) error {
	return s.store.RevokeAPIKey(ctx, id, userID)
}

// Resolve authenticates a raw Bearer value of the form gtpl_…: it returns
// the owning user and the key when the hash matches and the key is live.
// Missing keys are ErrNotFound; wrong hashes are indistinguishable from
// missing keys by prefix collision behavior (constant-time compare).
func (s *APIKeysService) Resolve(ctx context.Context, raw string) (model.APIKey, model.User, error) {
	if !strings.HasPrefix(raw, APIKeyPrefix) {
		return model.APIKey{}, model.User{}, repository.ErrNotFound
	}
	prefix := raw[:min(len(raw), len(APIKeyPrefix)+6)]
	stored, keyHash, err := s.store.GetAPIKeyByPrefix(ctx, prefix)
	if err != nil {
		return model.APIKey{}, model.User{}, repository.ErrNotFound
	}
	if stored.ExpiresAt != nil && time.Now().After(*stored.ExpiresAt) {
		return model.APIKey{}, model.User{}, repository.ErrNotFound
	}
	sum := sha256.Sum256([]byte(raw))
	if subtle.ConstantTimeCompare([]byte(encodeHash(sum)), []byte(keyHash)) != 1 {
		return model.APIKey{}, model.User{}, repository.ErrNotFound
	}
	_ = s.store.TouchAPIKey(ctx, stored.ID)
	// The API key resolves to its owner; the caller loads the user.
	return stored, model.User{ID: stored.UserID}, nil
}

// ScopesAllow reports whether the key's scopes cover obj:act.
func ScopesAllow(scopes []string, obj, act string) bool {
	if len(scopes) == 0 {
		return false
	}
	for _, scope := range scopes {
		if scope == obj+":"+act || scope == obj+":*" {
			return true
		}
	}
	return false
}

func encodeHash(sum [sha256.Size]byte) string {
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
