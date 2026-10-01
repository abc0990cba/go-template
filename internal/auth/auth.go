// Package auth owns credentials and sessions: registration, login, session
// resolution, impersonation, OAuth identity linking, and the Argon2id
// password hashing every credential path shares.
package auth

import (
	"context"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/cache"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/oauth"
	"github.com/your-org/go-template/internal/users"
)

// Identity links one external identity (password, google, github, facebook,
// twitter) to a user; (provider, provider_subject) is globally unique.
type Identity struct {
	UserID          string    `json:"user_id"`
	Provider        string    `json:"provider"`
	ProviderSubject string    `json:"provider_subject"`
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	AvatarURL       string    `json:"avatar_url"`
	LinkedAt        time.Time `json:"linked_at"`
	LastLoginAt     time.Time `json:"last_login_at"`
}

// OAuthState is a single-use CSRF value for the OAuth redirect round trip.
// CodeVerifier carries the RFC 7636 verifier for providers that mandate
// PKCE (Twitter); it stays empty for the others.
type OAuthState struct {
	State         string    `json:"state"`
	Provider      string    `json:"provider"`
	UserID        string    `json:"user_id"`
	RedirectAfter string    `json:"redirect_after"`
	CodeVerifier  string    `json:"code_verifier"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// Session is the resolution of an opaque session token: the user it
// belongs to and, for impersonation sessions, the admin behind it.
type Session struct {
	UserID         string
	ImpersonatedBy string
}

// SessionInfo describes one active session of a user. ID is a short prefix
// of the opaque token — enough to tell sessions apart, useless to resume one.
type SessionInfo struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ProviderInfo tells the UI whether an identity provider is configured,
// so link buttons render disabled instead of failing at click time.
type ProviderInfo struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}

// Store persists users, credentials, identities, sessions, and OAuth
// state for the auth flows.
type Store interface {
	CreateUser(ctx context.Context, user users.User, passwordHash string) error
	GetUser(ctx context.Context, id string) (users.User, error)
	GetUserByEmail(ctx context.Context, email string) (users.User, string, error)
	CreateSession(ctx context.Context, token, userID, impersonatedBy string, ttl time.Duration) error
	GetSession(ctx context.Context, token string, ttl time.Duration) (Session, error)
	DeleteSession(ctx context.Context, token string) error
	DeleteSessionsForUser(ctx context.Context, userID string) error
	LinkIdentity(ctx context.Context, identity Identity) error
	GetIdentity(ctx context.Context, provider, subject string) (Identity, error)
	UnlinkIdentity(ctx context.Context, userID, provider string) error
	ListIdentities(ctx context.Context, userID string) ([]Identity, error)
	ListSessions(ctx context.Context, userID string) ([]SessionInfo, error)
	SaveOAuthState(ctx context.Context, state OAuthState) error
	ConsumeOAuthState(ctx context.Context, state string, now time.Time) (OAuthState, error)
	// SetPasswordCredential upserts the password credential; "" removes it.
	SetPasswordCredential(ctx context.Context, userID, passwordHash string) error
}

// Service implements registration, login, and session resolution on top
// of a store, with Argon2id password hashing.
type Service struct {
	store      Store
	events     analytics.Sink
	sessionTTL time.Duration
	now        kernel.Clock
	// cache is optional: the user record behind a live session is the
	// hottest read in the app (every request resolves it). Invalidated by
	// the user service on writes — both share the same keys.
	cache cache.Cache
	group singleflight.Group
}

// NewService wires the auth service; the session TTL is the sliding expiry
// every session read renews.
func NewService(store Store, events analytics.Sink, sessionTTL time.Duration) *Service {
	return &Service{store: store, events: events, sessionTTL: sessionTTL, now: kernel.SystemClock}
}

// WithCache attaches the read-through user cache.
func (s *Service) WithCache(c cache.Cache) *Service { s.cache = c; return s }

// WithClock replaces the wall clock; tests walk time deterministically.
func (s *Service) WithClock(now kernel.Clock) *Service { s.now = now; return s }

// emit records an analytics event attributed to the acting user when a sink
// is configured.
func (s *Service) emit(ctx context.Context, event, userID string) {
	if s.events != nil {
		s.events.AddEvent(ctx, event, userID)
	}
}

// Register creates a user with a hashed password, opens a session, and
// returns the user and the session token.
func (s *Service) Register(ctx context.Context, email, name, password string) (users.User, string, error) {
	if !strings.Contains(email, "@") || len(password) < 8 {
		return users.User{}, "", kernel.Invalid("email must be valid and password must be at least 8 characters")
	}
	now := time.Now()
	user := users.User{ID: kernel.NewID(), Email: strings.ToLower(email), Name: name, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateUser(ctx, user, hashPassword(password)); err != nil {
		return users.User{}, "", err
	}
	token := kernel.NewID()
	if err := s.store.CreateSession(ctx, token, user.ID, "", s.sessionTTL); err != nil {
		return users.User{}, "", err
	}
	s.emit(ctx, "user_registered", user.ID)
	return user, token, nil
}

// Login verifies the password and opens a session. Unknown emails and wrong
// passwords both produce ErrInvalidCredentials.
func (s *Service) Login(ctx context.Context, email, password string) (users.User, string, error) {
	user, hash, err := s.store.GetUserByEmail(ctx, strings.ToLower(email))
	if err != nil || !verifyPassword(hash, password) {
		return users.User{}, "", kernel.ErrInvalidCredentials
	}
	token := kernel.NewID()
	if err := s.store.CreateSession(ctx, token, user.ID, "", s.sessionTTL); err != nil {
		return users.User{}, "", err
	}
	s.emit(ctx, "user_logged_in", user.ID)
	return user, token, nil
}

// Logout revokes a session token; a missing token is not an error here.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.store.DeleteSession(ctx, token)
}

// CurrentUser resolves the session token to its user; reading also slides
// the session expiry. The user record itself is served through the shared
// user cache (sessions stay uncached: they slide and revoke).
func (s *Service) CurrentUser(ctx context.Context, token string) (users.User, error) {
	session, err := s.store.GetSession(ctx, token, s.sessionTTL)
	if err != nil {
		return users.User{}, err
	}
	return cache.CachedGetOrLoad(ctx, s.cache, nil, &s.group, cache.UserCacheKey(session.UserID), cache.CacheTTLUser, func(ctx context.Context) (users.User, error) {
		return s.store.GetUser(ctx, session.UserID)
	})
}

// CurrentSession resolves the token with its impersonation context.
func (s *Service) CurrentSession(ctx context.Context, token string) (Session, error) {
	return s.store.GetSession(ctx, token, s.sessionTTL)
}

// Impersonate opens a session for the target on behalf of the admin. The
// admin's own sessions stay untouched; stopping creates a fresh one.
func (s *Service) Impersonate(ctx context.Context, adminID, targetID string) (string, error) {
	target, err := s.store.GetUser(ctx, targetID)
	if err != nil {
		return "", err
	}
	token := kernel.NewID()
	if err := s.store.CreateSession(ctx, token, target.ID, adminID, s.sessionTTL); err != nil {
		return "", err
	}
	return token, nil
}

// StopImpersonation revokes the impersonated session and issues a fresh
// session for the admin, returning the new token.
func (s *Service) StopImpersonation(ctx context.Context, token string) (string, error) {
	session, err := s.store.GetSession(ctx, token, s.sessionTTL)
	if err != nil {
		return "", err
	}
	if session.ImpersonatedBy == "" {
		return "", kernel.Invalid("this session is not impersonating")
	}
	if err := s.store.DeleteSession(ctx, token); err != nil {
		return "", err
	}
	adminToken := kernel.NewID()
	if err := s.store.CreateSession(ctx, adminToken, session.ImpersonatedBy, "", s.sessionTTL); err != nil {
		return "", err
	}
	return adminToken, nil
}

// UserByEmail resolves a user by email (case-insensitive), independent of
// any session — used by seeding and account flows.
func (s *Service) UserByEmail(ctx context.Context, email string) (users.User, error) {
	user, _, err := s.store.GetUserByEmail(ctx, strings.ToLower(email))
	return user, err
}

// LogoutAll revokes every session of the user (for example, after a
// password change or from the profile page).
func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	return s.store.DeleteSessionsForUser(ctx, userID)
}

// SaveState stores a single-use OAuth state with its expiry.
func (s *Service) SaveState(ctx context.Context, state OAuthState) error {
	return s.store.SaveOAuthState(ctx, state)
}

// ConsumeState validates and removes an OAuth state; consumed or expired
// states are not usable again.
func (s *Service) ConsumeState(ctx context.Context, state string) (OAuthState, error) {
	return s.store.ConsumeOAuthState(ctx, state, s.now())
}

// LinkIdentity attaches a verified provider profile to the user. The
// (provider, subject) pair comes from the provider, never from user input.
func (s *Service) LinkIdentity(ctx context.Context, userID string, profile oauth.Profile) error {
	switch profile.Provider {
	case "google", "github", "facebook", "twitter":
	default:
		return kernel.Invalid("provider must be one of google, github, facebook, twitter")
	}
	err := s.store.LinkIdentity(ctx, Identity{
		UserID:          userID,
		Provider:        profile.Provider,
		ProviderSubject: profile.ProviderSubject,
		Email:           profile.Email,
		Name:            profile.Name,
		AvatarURL:       profile.AvatarURL,
	})
	if err == nil {
		s.emit(ctx, "oauth_linked", userID)
	}
	return err
}

// UnlinkProvider detaches the user's identity for the provider.
func (s *Service) UnlinkProvider(ctx context.Context, userID, provider string) error {
	return s.store.UnlinkIdentity(ctx, userID, provider)
}

// Identities lists the identities linked to the user.
func (s *Service) Identities(ctx context.Context, userID string) ([]Identity, error) {
	return s.store.ListIdentities(ctx, userID)
}

// Sessions lists the user's active sessions for the profile page.
func (s *Service) Sessions(ctx context.Context, userID string) ([]SessionInfo, error) {
	return s.store.ListSessions(ctx, userID)
}
