package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/invitations"
	"github.com/your-org/go-template/internal/kernel"
)

// CreateOrgInvitation appends a pending invitation.
func (s *MemoryStore) CreateOrgInvitation(_ context.Context, invitation invitations.Invitation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invitations = append(s.invitations, invitation)
	return nil
}

// GetOrgInvitationByHash resolves an invitation by its token hash.
func (s *MemoryStore) GetOrgInvitationByHash(_ context.Context, tokenHash string) (invitations.Invitation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, invitation := range s.invitations {
		if invitation.TokenHash == tokenHash {
			return invitation, nil
		}
	}
	return invitations.Invitation{}, fmt.Errorf("%w: invitation not found", kernel.ErrNotFound)
}

// GetOrgInvitation returns one invitation by id.
func (s *MemoryStore) GetOrgInvitation(_ context.Context, id string) (invitations.Invitation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, invitation := range s.invitations {
		if invitation.ID == id {
			return invitation, nil
		}
	}
	return invitations.Invitation{}, fmt.Errorf("%w: invitation not found", kernel.ErrNotFound)
}

// ListOrgInvitations pages one org's invitations newest first.
func (s *MemoryStore) ListOrgInvitations(_ context.Context, orgID string, limit, offset int) ([]invitations.Invitation, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var orgInvitations []invitations.Invitation
	for _, invitation := range s.invitations {
		if invitation.OrgID == orgID {
			orgInvitations = append(orgInvitations, invitation)
		}
	}
	total := int64(len(orgInvitations))
	if offset >= len(orgInvitations) {
		return []invitations.Invitation{}, total, nil
	}
	end := offset + limit
	if end > len(orgInvitations) {
		end = len(orgInvitations)
	}
	return orgInvitations[offset:end], total, nil
}

// ListPendingOrgInvitationsByEmail returns the pending invitations addressed to an email.
func (s *MemoryStore) ListPendingOrgInvitationsByEmail(_ context.Context, email string) ([]invitations.Invitation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var out []invitations.Invitation
	for _, invitation := range s.invitations {
		if strings.EqualFold(invitation.Email, email) && invitation.Status == "pending" && invitation.ExpiresAt.After(now) {
			out = append(out, invitation)
		}
	}
	return out, nil
}

// SetOrgInvitationStatus records an accept/revoke/expiry transition.
func (s *MemoryStore) SetOrgInvitationStatus(_ context.Context, id, status string, acceptedAt *time.Time, acceptedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, invitation := range s.invitations {
		if invitation.ID == id {
			s.invitations[i].Status = status
			if acceptedAt != nil {
				s.invitations[i].AcceptedAt = acceptedAt
			}
			if acceptedBy != "" {
				s.invitations[i].AcceptedBy = acceptedBy
			}
			return nil
		}
	}
	return fmt.Errorf("%w: invitation not found", kernel.ErrNotFound)
}

// ExpireOldOrgInvitations marks every invitation past its expiry as expired.
func (s *MemoryStore) ExpireOldOrgInvitations(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i, invitation := range s.invitations {
		if invitation.Status == "pending" && !invitation.ExpiresAt.After(now) {
			s.invitations[i].Status = "expired"
		}
	}
	return nil
}

var _ invitations.Store = (*MemoryStore)(nil)
