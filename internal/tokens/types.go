package tokens

import (
	"context"
	"time"

	"github.com/your-org/go-template/internal/users"
)

// RefreshToken is one link in a rotation chain (family). Only the sha256 of
// the token is stored; RevokedAt is set on rotation, and a *presented*
// revoked token proves the family leaked.
type RefreshToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	FamilyID   string     `json:"family_id"`
	TokenHash  string     `json:"-"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	ReplacedBy string     `json:"replaced_by,omitempty"`
	UserAgent  string     `json:"user_agent,omitempty"`
	IP         string     `json:"ip,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Store persists refresh-token families for the JWT grant.
type Store interface {
	CreateRefreshToken(ctx context.Context, token RefreshToken) error
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id string) error
	RevokeRefreshFamily(ctx context.Context, familyID string) error
	RevokeRefreshTokensForUser(ctx context.Context, userID string) error
	DeleteExpiredRefreshTokens(ctx context.Context) error
}

// Auth is the narrow consumer-side view of the auth flows the grant needs:
// credential lookup by email and user fetch by id. The argon2 password
// verifier is injected separately (the verify func param on NewService).
type Auth interface {
	GetUser(ctx context.Context, id string) (users.User, error)
	GetUserByEmail(ctx context.Context, email string) (users.User, string, error)
}
