package invitations

import (
	"context"
	"time"
)

// Store persists org membership invitations.
type Store interface {
	CreateOrgInvitation(ctx context.Context, invitation Invitation) error
	GetOrgInvitationByHash(ctx context.Context, tokenHash string) (Invitation, error)
	GetOrgInvitation(ctx context.Context, id string) (Invitation, error)
	ListOrgInvitations(ctx context.Context, orgID string, limit, offset int) ([]Invitation, int64, error)
	ListPendingOrgInvitationsByEmail(ctx context.Context, email string) ([]Invitation, error)
	SetOrgInvitationStatus(ctx context.Context, id, status string, acceptedAt *time.Time, acceptedBy string) error
	ExpireOldOrgInvitations(ctx context.Context) error
}
