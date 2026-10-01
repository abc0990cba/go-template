package email

import (
	"context"
	"time"

	"github.com/your-org/go-template/internal/users"
)

// Token is one single-use, hashed email token (reset or verify). Only the
// hash is stored; the raw token exists solely inside the email link.
type Token struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Kind      string     `json:"kind"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// Store persists single-use hashed email tokens.
type Store interface {
	CreateEmailToken(ctx context.Context, token Token, tokenHash string) error
	GetEmailTokenByHash(ctx context.Context, tokenHash string) (Token, error)
	MarkEmailTokenUsed(ctx context.Context, id string) error
}

// AuthWriter is the slice of the auth feature the email flows need; the
// assembly passes the auth service, which satisfies it structurally.
type AuthWriter interface {
	GetUserByEmail(ctx context.Context, email string) (users.User, string, error)
	SetPasswordCredential(ctx context.Context, userID, passwordHash string) error
	DeleteSessionsForUser(ctx context.Context, userID string) error
}
