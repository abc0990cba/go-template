package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/your-org/go-template/internal/model"
)

// ErrInvalidCredentials is returned for any failed login, deliberately the
// same error for unknown email and wrong password so the API cannot be used
// to enumerate accounts.
var ErrInvalidCredentials = errors.New("invalid credentials")

// AuthService implements registration, login, and session resolution on top
// of a store, with Argon2id password hashing.
type AuthService struct {
	store AuthRepo
}

// Register creates a user with a hashed password, opens a session, and
// returns the user and the session token.
func (s *AuthService) Register(ctx context.Context, email, name, password string) (model.User, string, error) {
	if !strings.Contains(email, "@") || len(password) < 8 {
		return model.User{}, "", validationError("email must be valid and password must be at least 8 characters")
	}
	now := time.Now()
	user := model.User{ID: newID(), Email: strings.ToLower(email), Name: name, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateUser(ctx, user, hashPassword(password)); err != nil {
		return model.User{}, "", err
	}
	token := newID()
	if err := s.store.CreateSession(ctx, token, user.ID); err != nil {
		return model.User{}, "", err
	}
	s.store.AddEvent(ctx, "user_registered")
	return user, token, nil
}

// Login verifies the password and opens a session. Unknown emails and wrong
// passwords both produce ErrInvalidCredentials.
func (s *AuthService) Login(ctx context.Context, email, password string) (model.User, string, error) {
	user, hash, err := s.store.GetUserByEmail(ctx, strings.ToLower(email))
	if err != nil || !verifyPassword(hash, password) {
		return model.User{}, "", ErrInvalidCredentials
	}
	token := newID()
	if err := s.store.CreateSession(ctx, token, user.ID); err != nil {
		return model.User{}, "", err
	}
	s.store.AddEvent(ctx, "user_logged_in")
	return user, token, nil
}

// Logout revokes a session token; a missing token is not an error here.
func (s *AuthService) Logout(ctx context.Context, token string) error {
	return s.store.DeleteSession(ctx, token)
}

// CurrentUser resolves the session token to its user.
func (s *AuthService) CurrentUser(ctx context.Context, token string) (model.User, error) {
	userID, err := s.store.GetSession(ctx, token)
	if err != nil {
		return model.User{}, err
	}
	return s.store.GetUser(ctx, userID)
}

// LinkProvider attaches a placeholder provider identity to the user. The
// real OAuth exchange is wired by the auth OpenSpec change.
func (s *AuthService) LinkProvider(ctx context.Context, userID, provider string) error {
	if provider != "google" && provider != "github" {
		return validationError("provider must be google or github")
	}
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	err = s.store.LinkIdentity(ctx, model.AuthIdentity{
		UserID: userID, Provider: provider, ProviderSubject: provider + ":" + strings.ToLower(user.Email), Email: user.Email,
	})
	if err == nil {
		s.store.AddEvent(ctx, "oauth_linked")
	}
	return err
}

// UnlinkProvider detaches the user's identity for the provider.
func (s *AuthService) UnlinkProvider(ctx context.Context, userID, provider string) error {
	return s.store.UnlinkIdentity(ctx, userID, provider)
}

// Identities lists the identities linked to the user.
func (s *AuthService) Identities(ctx context.Context, userID string) ([]model.AuthIdentity, error) {
	return s.store.ListIdentities(ctx, userID)
}

func hashPassword(password string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return base64.RawStdEncoding.EncodeToString(salt) + ":" + base64.RawStdEncoding.EncodeToString(hash)
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, ":")
	if len(parts) != 2 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
