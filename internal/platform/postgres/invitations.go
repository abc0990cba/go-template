package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/your-org/go-template/internal/invitations"
	"github.com/your-org/go-template/internal/platform/postgres/db"
)

func (s *Store) CreateOrgInvitation(ctx context.Context, invitation invitations.Invitation) error {
	return mapErr(s.q.CreateOrgInvitation(ctx, db.CreateOrgInvitationParams{
		ID:        invitation.ID,
		OrgID:     invitation.OrgID,
		Email:     invitation.Email,
		Role:      invitation.Role,
		InvitedBy: invitation.InvitedBy,
		TokenHash: invitation.TokenHash,
		ExpiresAt: invitation.ExpiresAt,
	}))
}

func (s *Store) GetOrgInvitationByHash(ctx context.Context, tokenHash string) (invitations.Invitation, error) {
	row, err := s.q.GetOrgInvitationByHash(ctx, tokenHash)
	if err != nil {
		return invitations.Invitation{}, mapErr(err)
	}
	return invitationFromRow(row), nil
}

func (s *Store) GetOrgInvitation(ctx context.Context, id string) (invitations.Invitation, error) {
	row, err := s.q.GetOrgInvitation(ctx, id)
	if err != nil {
		return invitations.Invitation{}, mapErr(err)
	}
	return invitationFromRow(row), nil
}

func (s *Store) ListOrgInvitations(ctx context.Context, orgID string, limit, offset int) ([]invitations.Invitation, int64, error) {
	rows, err := s.q.ListOrgInvitations(ctx, db.ListOrgInvitationsParams{
		OrgID:  orgID,
		Offset: int32(offset),
		Limit:  int32(limit),
	})
	if err != nil {
		return nil, 0, mapErr(err)
	}
	total, err := s.q.CountOrgInvitations(ctx, orgID)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	out := make([]invitations.Invitation, 0, len(rows))
	for _, row := range rows {
		out = append(out, invitationFromRow(row))
	}
	return out, total, nil
}

func (s *Store) ListPendingOrgInvitationsByEmail(ctx context.Context, email string) ([]invitations.Invitation, error) {
	rows, err := s.q.ListPendingOrgInvitationsByEmail(ctx, email)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]invitations.Invitation, 0, len(rows))
	for _, row := range rows {
		out = append(out, invitationFromRow(row))
	}
	return out, nil
}

func (s *Store) SetOrgInvitationStatus(ctx context.Context, id, status string, acceptedAt *time.Time, acceptedBy string) error {
	params := db.SetOrgInvitationStatusParams{Status: status, ID: id}
	if acceptedAt != nil {
		params.AcceptedAt = pgtype.Timestamptz{Time: *acceptedAt, Valid: true}
	}
	if acceptedBy != "" {
		params.AcceptedBy = acceptedBy
	}
	return mapErr(s.q.SetOrgInvitationStatus(ctx, params))
}

func (s *Store) ExpireOldOrgInvitations(ctx context.Context) error {
	return mapErr(s.q.ExpireOldOrgInvitations(ctx))
}

func invitationFromRow(row db.OrgInvitation) invitations.Invitation {
	out := invitations.Invitation{
		ID:         row.ID,
		OrgID:      row.OrgID,
		Email:      row.Email,
		Role:       row.Role,
		InvitedBy:  row.InvitedBy,
		TokenHash:  row.TokenHash,
		Status:     row.Status,
		ExpiresAt:  row.ExpiresAt,
		AcceptedBy: row.AcceptedBy,
		CreatedAt:  row.CreatedAt,
	}
	if row.AcceptedAt.Valid {
		t := row.AcceptedAt.Time
		out.AcceptedAt = &t
	}
	return out
}

var _ invitations.Store = (*Store)(nil)
