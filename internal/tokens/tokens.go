// Package tokens issues JWT access tokens with rotating refresh tokens for
// non-browser clients: the OAuth-style grant endpoints under /auth/token.
package tokens

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/mfa"
	"github.com/your-org/go-template/internal/users"
)

// Token errors; handlers map them to statuses.
var (
	// ErrAuthDisabled reports that no JWT secret is configured.
	ErrAuthDisabled = errors.New("token authentication is not configured")
	// ErrInvalidGrant covers bad credentials, unknown/expired refresh
	// tokens, and malformed grants — deliberately uniform.
	ErrInvalidGrant = errors.New("invalid grant")
	// ErrTokenReuse reports a rotated refresh token being presented again;
	// the whole family has been revoked as a breach response.
	ErrTokenReuse = errors.New("refresh token reuse detected")
)

// Token grant defaults; config can override the TTLs.
const (
	DefaultAccessTokenTTL  = 15 * time.Minute
	DefaultRefreshTokenTTL = 30 * 24 * time.Hour
	// Issuer stamps the access tokens; verification requires it.
	Issuer = "go-template"
)

// Service issues JWT access tokens with rotating refresh tokens for
// non-browser clients. AuthZ stays server-side (Casbin) per request — the
// access token carries only identity claims, never roles. Refresh tokens
// rotate on every use; presenting a rotated token revokes its whole family
// (the standard reuse-detection breach response).
type Service struct {
	store      Store
	auth       Auth
	events     analytics.Sink
	mfa        *mfa.Service
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	verify     func(encoded, password string) bool
}

// NewService wires the grant. An empty secret keeps the endpoints
// answering ErrAuthDisabled (the feature is opt-in per deployment). The
// argon2 password verifier is injected (it lives with the auth feature —
// the email hash-param pattern).
func NewService(store Store, auth Auth, mfa *mfa.Service, events analytics.Sink, secret string, accessTTL, refreshTTL time.Duration, verify func(encoded, password string) bool) *Service {
	return &Service{
		store: store, auth: auth, mfa: mfa, events: events,
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		verify:     verify,
	}
}

// GrantResult is the token-endpoint response payload.
type GrantResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	TokenType    string
}

// emit records a login event when a sink exists.
func (s *Service) emit(ctx context.Context, event, userID string) {
	if s.events != nil {
		s.events.AddEvent(ctx, event, userID)
	}
}

// PasswordGrant exchanges credentials for tokens. MFA-enrolled accounts get
// the same challenge bridge as the browser login: the grant answers with the
// challenge token and CompleteChallengeGrant finishes it.
func (s *Service) PasswordGrant(ctx context.Context, email, password, userAgent, ip string) (users.User, *GrantResult, string, error) {
	if len(s.secret) == 0 {
		return users.User{}, nil, "", ErrAuthDisabled
	}
	user, hash, err := s.auth.GetUserByEmail(ctx, strings.ToLower(email))
	if err != nil || hash == "" || !s.verify(hash, password) {
		return users.User{}, nil, "", ErrInvalidGrant
	}
	if s.mfa != nil && s.mfa.Enabled(ctx, user.ID) {
		challenge, err := s.mfa.BeginChallenge(ctx, user.ID)
		if err != nil {
			return users.User{}, nil, "", err
		}
		return user, nil, challenge, nil
	}
	if s.mfa.RequiredAndMissing(ctx, user.ID) {
		return users.User{}, nil, "", mfa.ErrEnrollmentRequired
	}
	grant, err := s.issue(ctx, user.ID, kernel.NewID(), userAgent, ip)
	if err != nil {
		return users.User{}, nil, "", err
	}
	s.emit(ctx, "user_logged_in", user.ID)
	return user, grant, "", nil
}

// CompleteChallengeGrant finishes an MFA login for a token client.
func (s *Service) CompleteChallengeGrant(ctx context.Context, challenge, code, userAgent, ip string) (users.User, *GrantResult, error) {
	if len(s.secret) == 0 {
		return users.User{}, nil, ErrAuthDisabled
	}
	if s.mfa == nil {
		return users.User{}, nil, ErrInvalidGrant
	}
	user, _, err := s.mfa.CompleteChallenge(ctx, challenge, code)
	if err != nil {
		return users.User{}, nil, err
	}
	grant, err := s.issue(ctx, user.ID, kernel.NewID(), userAgent, ip)
	if err != nil {
		return users.User{}, nil, err
	}
	s.emit(ctx, "user_logged_in", user.ID)
	return user, grant, nil
}

// RefreshGrant rotates the presented refresh token. A revoked token reaching
// here means its family leaked: every token in the family dies immediately.
func (s *Service) RefreshGrant(ctx context.Context, refreshToken, userAgent, ip string) (users.User, *GrantResult, error) {
	if len(s.secret) == 0 {
		return users.User{}, nil, ErrAuthDisabled
	}
	if refreshToken == "" {
		return users.User{}, nil, ErrInvalidGrant
	}
	stored, err := s.store.GetRefreshTokenByHash(ctx, hashToken(refreshToken))
	if err != nil {
		return users.User{}, nil, ErrInvalidGrant
	}
	if stored.RevokedAt != nil {
		_ = s.store.RevokeRefreshFamily(ctx, stored.FamilyID)
		return users.User{}, nil, ErrTokenReuse
	}
	if time.Now().After(stored.ExpiresAt) {
		return users.User{}, nil, ErrInvalidGrant
	}
	if err := s.store.RevokeRefreshToken(ctx, stored.ID); err != nil {
		return users.User{}, nil, err
	}
	grant, err := s.issue(ctx, stored.UserID, stored.FamilyID, userAgent, ip)
	if err != nil {
		// The old token is already dead; the client must re-authenticate.
		return users.User{}, nil, err
	}
	user, err := s.auth.GetUser(ctx, stored.UserID)
	if err != nil {
		return users.User{}, nil, ErrInvalidGrant
	}
	return user, grant, nil
}

// Revoke kills the presented refresh token; unknown tokens vanish silently
// so the endpoint cannot probe which tokens exist.
func (s *Service) Revoke(ctx context.Context, refreshToken string) error {
	if len(s.secret) == 0 {
		return ErrAuthDisabled
	}
	stored, err := s.store.GetRefreshTokenByHash(ctx, hashToken(refreshToken))
	if err != nil {
		return nil
	}
	return s.store.RevokeRefreshToken(ctx, stored.ID)
}

// RevokeAllForUser drops every live refresh token (logout-everywhere for
// token clients).
func (s *Service) RevokeAllForUser(ctx context.Context, userID string) error {
	if len(s.secret) == 0 {
		return ErrAuthDisabled
	}
	return s.store.RevokeRefreshTokensForUser(ctx, userID)
}

// Prune drops long-expired refresh tokens; the worker calls it on a schedule.
func (s *Service) Prune(ctx context.Context) error {
	if len(s.secret) == 0 {
		return nil
	}
	return s.store.DeleteExpiredRefreshTokens(ctx)
}

// AccessConfigured reports whether token auth is enabled for this deployment.
func (s *Service) AccessConfigured() bool { return len(s.secret) > 0 }

// issue mints an access JWT plus a new refresh token in the given family.
func (s *Service) issue(ctx context.Context, userID, familyID, userAgent, ip string) (*GrantResult, error) {
	now := time.Now()
	access, err := s.signAccess(userID, now)
	if err != nil {
		return nil, err
	}
	refresh := kernel.NewID()
	if err := s.store.CreateRefreshToken(ctx, RefreshToken{
		ID:        kernel.NewID(),
		UserID:    userID,
		FamilyID:  familyID,
		TokenHash: hashToken(refresh),
		ExpiresAt: now.Add(s.refreshTTL),
		UserAgent: userAgent,
		IP:        ip,
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	return &GrantResult{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(s.accessTTL.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

// signAccess produces an HS256 JWT with identity-only claims. AuthZ is
// resolved server-side per request, so roles and orgs never ride in the
// token and permission changes apply immediately.
func (s *Service) signAccess(userID string, now time.Time) (string, error) {
	claims := jwt.MapClaims{
		"iss": Issuer,
		"sub": userID,
		"typ": "access",
		"iat": now.Unix(),
		"exp": now.Add(s.accessTTL).Unix(),
		"jti": kernel.NewID(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// hashToken is the one place refresh tokens are hashed — sha256 hex, the
// same scheme the mfa and email features use. Copied, not shared: the
// features stay decoupled (the invitation flow keeps its own copy until it
// migrates).
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// VerifyAccess parses and validates an access token, returning the user id.
func (s *Service) VerifyAccess(tokenString string) (string, error) {
	if len(s.secret) == 0 {
		return "", ErrAuthDisabled
	}
	parsed, err := jwt.Parse(tokenString, func(_ *jwt.Token) (any, error) {
		return s.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuer(Issuer))
	if err != nil {
		return "", ErrInvalidGrant
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || !parsed.Valid {
		return "", ErrInvalidGrant
	}
	if typ, _ := claims["typ"].(string); typ != "access" {
		return "", ErrInvalidGrant
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", ErrInvalidGrant
	}
	return sub, nil
}
