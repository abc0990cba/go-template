package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/your-org/go-template/internal/model"
)

// Token errors; handlers map them to statuses.
var (
	// ErrTokenAuthDisabled reports that no JWT secret is configured.
	ErrTokenAuthDisabled = errors.New("token authentication is not configured")
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
	// JWTIssuer stamps the access tokens; verification requires it.
	JWTIssuer = "go-template"
)

// TokenService issues JWT access tokens with rotating refresh tokens for
// non-browser clients. AuthZ stays server-side (Casbin) per request — the
// access token carries only identity claims, never roles. Refresh tokens
// rotate on every use; presenting a rotated token revokes its whole family
// (the standard reuse-detection breach response).
type TokenService struct {
	store      TokenRepo
	auth       AuthRepo
	events     EventSink
	mfa        *MFAService
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewTokenService wires the grant. An empty secret keeps the endpoints
// answering ErrTokenAuthDisabled (the feature is opt-in per deployment).
func NewTokenService(store TokenRepo, auth AuthRepo, mfa *MFAService, events EventSink, secret string, accessTTL, refreshTTL time.Duration) *TokenService {
	return &TokenService{
		store: store, auth: auth, mfa: mfa, events: events,
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
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
func (s *TokenService) emit(ctx context.Context, event, userID string) {
	if s.events != nil {
		s.events.AddEvent(ctx, event, userID)
	}
}

// PasswordGrant exchanges credentials for tokens. MFA-enrolled accounts get
// the same challenge bridge as the browser login: the grant answers with the
// challenge token and CompleteChallengeGrant finishes it.
func (s *TokenService) PasswordGrant(ctx context.Context, email, password, userAgent, ip string) (model.User, *GrantResult, string, error) {
	if len(s.secret) == 0 {
		return model.User{}, nil, "", ErrTokenAuthDisabled
	}
	user, hash, err := s.auth.GetUserByEmail(ctx, strings.ToLower(email))
	if err != nil || hash == "" || !verifyPassword(hash, password) {
		return model.User{}, nil, "", ErrInvalidGrant
	}
	if s.mfa != nil && s.mfa.Enabled(ctx, user.ID) {
		challenge, err := s.mfa.BeginChallenge(ctx, user.ID)
		if err != nil {
			return model.User{}, nil, "", err
		}
		return user, nil, challenge, nil
	}
	if s.mfa.RequiredAndMissing(ctx, user.ID) {
		return model.User{}, nil, "", ErrMFAEnrollmentRequired
	}
	grant, err := s.issue(ctx, user.ID, newID(), userAgent, ip)
	if err != nil {
		return model.User{}, nil, "", err
	}
	s.emit(ctx, "user_logged_in", user.ID)
	return user, grant, "", nil
}

// CompleteChallengeGrant finishes an MFA login for a token client.
func (s *TokenService) CompleteChallengeGrant(ctx context.Context, challenge, code, userAgent, ip string) (model.User, *GrantResult, error) {
	if len(s.secret) == 0 {
		return model.User{}, nil, ErrTokenAuthDisabled
	}
	if s.mfa == nil {
		return model.User{}, nil, ErrInvalidGrant
	}
	user, _, err := s.mfa.CompleteChallenge(ctx, challenge, code)
	if err != nil {
		return model.User{}, nil, err
	}
	grant, err := s.issue(ctx, user.ID, newID(), userAgent, ip)
	if err != nil {
		return model.User{}, nil, err
	}
	s.emit(ctx, "user_logged_in", user.ID)
	return user, grant, nil
}

// RefreshGrant rotates the presented refresh token. A revoked token reaching
// here means its family leaked: every token in the family dies immediately.
func (s *TokenService) RefreshGrant(ctx context.Context, refreshToken, userAgent, ip string) (model.User, *GrantResult, error) {
	if len(s.secret) == 0 {
		return model.User{}, nil, ErrTokenAuthDisabled
	}
	if refreshToken == "" {
		return model.User{}, nil, ErrInvalidGrant
	}
	stored, err := s.store.GetRefreshTokenByHash(ctx, hashToken(refreshToken))
	if err != nil {
		return model.User{}, nil, ErrInvalidGrant
	}
	if stored.RevokedAt != nil {
		_ = s.store.RevokeRefreshFamily(ctx, stored.FamilyID)
		return model.User{}, nil, ErrTokenReuse
	}
	if time.Now().After(stored.ExpiresAt) {
		return model.User{}, nil, ErrInvalidGrant
	}
	if err := s.store.RevokeRefreshToken(ctx, stored.ID); err != nil {
		return model.User{}, nil, err
	}
	grant, err := s.issue(ctx, stored.UserID, stored.FamilyID, userAgent, ip)
	if err != nil {
		// The old token is already dead; the client must re-authenticate.
		return model.User{}, nil, err
	}
	user, err := s.auth.GetUser(ctx, stored.UserID)
	if err != nil {
		return model.User{}, nil, ErrInvalidGrant
	}
	return user, grant, nil
}

// Revoke kills the presented refresh token; unknown tokens vanish silently
// so the endpoint cannot probe which tokens exist.
func (s *TokenService) Revoke(ctx context.Context, refreshToken string) error {
	if len(s.secret) == 0 {
		return ErrTokenAuthDisabled
	}
	stored, err := s.store.GetRefreshTokenByHash(ctx, hashToken(refreshToken))
	if err != nil {
		return nil
	}
	return s.store.RevokeRefreshToken(ctx, stored.ID)
}

// RevokeAllForUser drops every live refresh token (logout-everywhere for
// token clients).
func (s *TokenService) RevokeAllForUser(ctx context.Context, userID string) error {
	if len(s.secret) == 0 {
		return ErrTokenAuthDisabled
	}
	return s.store.RevokeRefreshTokensForUser(ctx, userID)
}

// Prune drops long-expired refresh tokens; the worker calls it on a schedule.
func (s *TokenService) Prune(ctx context.Context) error {
	if len(s.secret) == 0 {
		return nil
	}
	return s.store.DeleteExpiredRefreshTokens(ctx)
}

// AccessConfigured reports whether token auth is enabled for this deployment.
func (s *TokenService) AccessConfigured() bool { return len(s.secret) > 0 }

// issue mints an access JWT plus a new refresh token in the given family.
func (s *TokenService) issue(ctx context.Context, userID, familyID, userAgent, ip string) (*GrantResult, error) {
	now := time.Now()
	access, err := s.signAccess(userID, now)
	if err != nil {
		return nil, err
	}
	refresh := newID()
	if err := s.store.CreateRefreshToken(ctx, model.RefreshToken{
		ID:        newID(),
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
func (s *TokenService) signAccess(userID string, now time.Time) (string, error) {
	claims := jwt.MapClaims{
		"iss": JWTIssuer,
		"sub": userID,
		"typ": "access",
		"iat": now.Unix(),
		"exp": now.Add(s.accessTTL).Unix(),
		"jti": newID(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// VerifyAccess parses and validates an access token, returning the user id.
func (s *TokenService) VerifyAccess(tokenString string) (string, error) {
	if len(s.secret) == 0 {
		return "", ErrTokenAuthDisabled
	}
	parsed, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		return s.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuer(JWTIssuer))
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
