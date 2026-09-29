package service

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/platform/mailer"
	"github.com/your-org/go-template/internal/repository"
)

// Invitation tuning.
const (
	InvitationTTL      = 7 * 24 * time.Hour
	InvitationsPageMax = 100
)

// InvitationsService runs the org membership invitation flow: admins invite
// an email with a role, the invitee accepts after signing in, and the
// membership + domain role grant land in one step. Emails go through the
// shared mailer (SMTP in production, log in dev); the raw token exists only
// in that email.
type InvitationsService struct {
	store   InvitationsRepo
	orgs    OrgsRepo
	auth    AuthRepo
	rbac    *RBACService
	mailer  mailer.Sender
	baseURL string
	logger  *slog.Logger
}

// NewInvitationsService wires the flow.
func NewInvitationsService(store InvitationsRepo, orgs OrgsRepo, auth AuthRepo, rbac *RBACService, sender mailer.Sender, baseURL string, logger *slog.Logger) *InvitationsService {
	return &InvitationsService{
		store: store, orgs: orgs, auth: auth, rbac: rbac, mailer: sender,
		baseURL: strings.TrimRight(baseURL, "/"), logger: logger,
	}
}

// Invite creates a pending invitation and emails the accept link. Uniform
// conflict errors avoid leaking whether the address is already a member or
// invited: callers see one "invitation not possible" answer.
func (s *InvitationsService) Invite(ctx context.Context, orgID, email, role, invitedBy string) (model.OrgInvitation, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if orgID == "" || email == "" || !strings.Contains(email, "@") {
		return model.OrgInvitation{}, "", validationError("organization and a valid email are required")
	}
	if _, err := s.orgs.GetOrg(ctx, orgID); err != nil {
		return model.OrgInvitation{}, "", err
	}
	if _, err := s.rbac.RoleByKey(role); err != nil {
		return model.OrgInvitation{}, "", validationError("unknown role " + role)
	}
	if user, _, err := s.auth.GetUserByEmail(ctx, email); err == nil {
		if member, merr := s.orgs.IsOrgMember(ctx, orgID, user.ID); merr == nil && member {
			return model.OrgInvitation{}, "", fmt.Errorf("%w: already a member", repository.ErrConflict)
		}
	}
	pending, err := s.store.ListPendingOrgInvitationsByEmail(ctx, email)
	if err == nil {
		for _, invitation := range pending {
			if invitation.OrgID == orgID {
				return model.OrgInvitation{}, "", fmt.Errorf("%w: invitation already pending", repository.ErrConflict)
			}
		}
	}
	token := newID()
	now := time.Now()
	invitation := model.OrgInvitation{
		ID:        newID(),
		OrgID:     orgID,
		Email:     email,
		Role:      role,
		InvitedBy: invitedBy,
		TokenHash: hashToken(token),
		Status:    model.InvitationPending,
		ExpiresAt: now.Add(InvitationTTL),
		CreatedAt: now,
	}
	if err := s.store.CreateOrgInvitation(ctx, invitation); err != nil {
		return model.OrgInvitation{}, "", err
	}
	link := fmt.Sprintf("%s/accept-invitation?token=%s", s.baseURL, token)
	s.sendInviteEmail(ctx, invitation, link)
	return invitation, token, nil
}

// Accept validates the token and lands the membership: org_members row plus
// the Casbin domain role grant. Expired or non-pending invitations are a
// uniform validation error.
func (s *InvitationsService) Accept(ctx context.Context, token, userID string) (model.OrgInvitation, error) {
	if token == "" {
		return model.OrgInvitation{}, validationError("invitation token is required")
	}
	invitation, err := s.store.GetOrgInvitationByHash(ctx, hashToken(token))
	if err != nil {
		return model.OrgInvitation{}, fmt.Errorf("%w: invitation not found", repository.ErrNotFound)
	}
	if invitation.Status != model.InvitationPending || time.Now().After(invitation.ExpiresAt) {
		return model.OrgInvitation{}, fmt.Errorf("%w: invitation is no longer valid", repository.ErrConflict)
	}
	// Idempotent membership insert: existing rows are fine.
	if err := s.orgs.AddOrgMember(ctx, model.OrgMember{OrgID: invitation.OrgID, UserID: userID, JoinedAt: time.Now()}); err != nil && !errors.Is(err, repository.ErrConflict) {
		return model.OrgInvitation{}, err
	}
	if err := s.rbac.AssignRole(userID, invitation.Role, invitation.OrgID); err != nil {
		return model.OrgInvitation{}, err
	}
	now := time.Now()
	if err := s.store.SetOrgInvitationStatus(ctx, invitation.ID, model.InvitationAccepted, &now, userID); err != nil {
		return model.OrgInvitation{}, err
	}
	invitation.Status = model.InvitationAccepted
	invitation.AcceptedAt = &now
	invitation.AcceptedBy = userID
	return invitation, nil
}

// Revoke withdraws a pending invitation (admin action).
func (s *InvitationsService) Revoke(ctx context.Context, orgID, invitationID string) error {
	invitation, err := s.store.GetOrgInvitation(ctx, invitationID)
	if err != nil {
		return err
	}
	if invitation.OrgID != orgID {
		return fmt.Errorf("%w: invitation not found", repository.ErrNotFound)
	}
	if invitation.Status != model.InvitationPending {
		return fmt.Errorf("%w: only pending invitations can be revoked", repository.ErrConflict)
	}
	return s.store.SetOrgInvitationStatus(ctx, invitationID, model.InvitationRevoked, nil, "")
}

// ForOrg lists one org's invitations (newest first).
func (s *InvitationsService) ForOrg(ctx context.Context, orgID string, limit, offset int) ([]model.OrgInvitation, int64, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > InvitationsPageMax {
		limit = InvitationsPageMax
	}
	return s.store.ListOrgInvitations(ctx, orgID, limit, offset)
}

// Mine lists the pending invitations addressed to the session's email.
func (s *InvitationsService) Mine(ctx context.Context, email string) ([]model.OrgInvitation, error) {
	return s.store.ListPendingOrgInvitationsByEmail(ctx, email)
}

func (s *InvitationsService) sendInviteEmail(ctx context.Context, invitation model.OrgInvitation, link string) {
	if s.mailer == nil {
		s.logger.Info("invitation created (no mailer configured)", "invitee", invitation.Email, "link", link)
		return
	}
	subject := "You are invited to join an organization"
	body := template.HTML(template.HTMLEscapeString(
		`You have been invited with the role "` + invitation.Role + `". ` +
			`Accept: <a href="` + link + `">` + link + `</a> ` +
			`(valid for 7 days; sign in or register first).`))
	if err := s.mailer.Send(ctx, mailer.Message{To: invitation.Email, Subject: subject, Body: body}); err != nil {
		s.logger.Error("invitation email failed", "err", err, "invitee", invitation.Email)
	}
}
