package postgres

import (
	"context"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/platform/postgres/db"
)

func (s *Store) CreateRefreshToken(ctx context.Context, token model.RefreshToken) error {
	return mapErr(s.q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		ID:        token.ID,
		UserID:    token.UserID,
		FamilyID:  token.FamilyID,
		TokenHash: token.TokenHash,
		ExpiresAt: token.ExpiresAt,
		UserAgent: token.UserAgent,
		Ip:        token.IP,
	}))
}

func (s *Store) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (model.RefreshToken, error) {
	row, err := s.q.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		return model.RefreshToken{}, mapErr(err)
	}
	out := model.RefreshToken{
		ID:         row.ID,
		UserID:     row.UserID,
		FamilyID:   row.FamilyID,
		TokenHash:  row.TokenHash,
		ExpiresAt:  row.ExpiresAt,
		ReplacedBy: row.ReplacedBy,
		UserAgent:  row.UserAgent,
		IP:         row.Ip,
		CreatedAt:  row.CreatedAt,
	}
	if row.RevokedAt.Valid {
		t := row.RevokedAt.Time
		out.RevokedAt = &t
	}
	return out, nil
}

func (s *Store) RevokeRefreshToken(ctx context.Context, id string) error {
	return mapErr(s.q.RevokeRefreshToken(ctx, id))
}

func (s *Store) RevokeRefreshFamily(ctx context.Context, familyID string) error {
	return mapErr(s.q.RevokeRefreshFamily(ctx, familyID))
}

func (s *Store) RevokeRefreshTokensForUser(ctx context.Context, userID string) error {
	return mapErr(s.q.RevokeRefreshTokensForUser(ctx, userID))
}

func (s *Store) DeleteExpiredRefreshTokens(ctx context.Context) error {
	return mapErr(s.q.DeleteExpiredRefreshTokens(ctx))
}
