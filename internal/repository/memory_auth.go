// Auth-store methods of the in-memory fake: password credentials,
// sessions, OAuth identities, and single-use OAuth state. The user CRUD
// they share (CreateUser/GetUser/GetUserByEmail) stays in memory.go — the
// users.Store port declares it too.
package repository

import (
	"context"
	"sort"
	"time"

	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/kernel"
)

// CreateSession maps an opaque session token to a user id with a TTL.
// impersonatedBy marks an impersonation session ("" for normal logins).
func (s *MemoryStore) CreateSession(ctx context.Context, token, userID, impersonatedBy string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = sessionEntry{userID: userID, impersonatedBy: impersonatedBy, expiresAt: time.Now().Add(ttl)}
	return nil
}

// GetSession returns the user id behind a session token and slides its
// expiry forward (renewal on use). Expired tokens are removed. It reports
// the impersonator, if any.
func (s *MemoryStore) GetSession(ctx context.Context, token string, ttl time.Duration) (auth.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.sessions[token]
	if !ok {
		return auth.Session{}, kernel.ErrNotFound
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.sessions, token)
		return auth.Session{}, kernel.ErrNotFound
	}
	entry.expiresAt = time.Now().Add(ttl)
	s.sessions[token] = entry
	return auth.Session{UserID: entry.userID, ImpersonatedBy: entry.impersonatedBy}, nil
}

// DeleteSession revokes a session token.
func (s *MemoryStore) DeleteSession(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
	return nil
}

// DeleteSessionsForUser revokes every session of the user (logout-all).
func (s *MemoryStore) DeleteSessionsForUser(ctx context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, entry := range s.sessions {
		if entry.userID == userID {
			delete(s.sessions, token)
		}
	}
	return nil
}

// SetPasswordCredential upserts the stored password hash; "" removes the
// credential entirely.
func (s *MemoryStore) SetPasswordCredential(ctx context.Context, userID, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return kernel.ErrNotFound
	}
	if passwordHash == "" {
		delete(s.passwords, user.Email)
		return nil
	}
	s.passwords[user.Email] = passwordHash
	return nil
}

// LinkIdentity attaches an external identity to a user. Relinking the same
// (provider, subject) to the same user is a no-op; a different user is a
// conflict.
func (s *MemoryStore) LinkIdentity(ctx context.Context, identity auth.Identity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.identities {
		if existing.Provider == identity.Provider && existing.ProviderSubject == identity.ProviderSubject {
			if existing.UserID == identity.UserID {
				return nil
			}
			return kernel.ErrConflict
		}
	}
	identity.LinkedAt = time.Now()
	if identity.LastLoginAt.IsZero() {
		identity.LastLoginAt = identity.LinkedAt
	}
	s.identities = append(s.identities, identity)
	return nil
}

// GetIdentity returns the identity for a (provider, subject) pair.
func (s *MemoryStore) GetIdentity(ctx context.Context, provider, subject string) (auth.Identity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, identity := range s.identities {
		if identity.Provider == provider && identity.ProviderSubject == subject {
			return identity, nil
		}
	}
	return auth.Identity{}, kernel.ErrNotFound
}

// UnlinkIdentity removes the identity a user holds with the given provider.
func (s *MemoryStore) UnlinkIdentity(ctx context.Context, userID, provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.identities[:0]
	for _, identity := range s.identities {
		if identity.UserID == userID && identity.Provider == provider {
			continue
		}
		next = append(next, identity)
	}
	s.identities = next
	return nil
}

// ListIdentities returns every identity linked to the user.
func (s *MemoryStore) ListIdentities(ctx context.Context, userID string) ([]auth.Identity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var identities []auth.Identity
	for _, identity := range s.identities {
		if identity.UserID == userID {
			identities = append(identities, identity)
		}
	}
	return identities, nil
}

// ListSessions returns the user's active sessions newest first, keyed by a
// short prefix of the opaque token.
func (s *MemoryStore) ListSessions(ctx context.Context, userID string) ([]auth.SessionInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var sessions []auth.SessionInfo
	for token, entry := range s.sessions {
		if entry.userID != userID || now.After(entry.expiresAt) {
			continue
		}
		sessions = append(sessions, auth.SessionInfo{
			ID:        tokenPrefix(token),
			CreatedAt: now, // the fake does not track creation time
			ExpiresAt: entry.expiresAt,
		})
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].ExpiresAt.After(sessions[j].ExpiresAt) })
	return sessions, nil
}

// SaveOAuthState remembers a single-use OAuth state value with its expiry.
func (s *MemoryStore) SaveOAuthState(ctx context.Context, state auth.OAuthState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.oauthStates[state.State] = state
	return nil
}

// ConsumeOAuthState removes and returns the state, or kernel.ErrNotFound when it is
// unknown or expired. A consumed state can never be used again.
func (s *MemoryStore) ConsumeOAuthState(ctx context.Context, state string, now time.Time) (auth.OAuthState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	parsed, ok := s.oauthStates[state]
	if !ok {
		return auth.OAuthState{}, kernel.ErrNotFound
	}
	delete(s.oauthStates, state)
	if now.After(parsed.ExpiresAt) {
		return auth.OAuthState{}, kernel.ErrNotFound
	}
	return parsed, nil
}
