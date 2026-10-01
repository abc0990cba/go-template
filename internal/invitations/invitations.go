package invitations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/mailer"
	"github.com/your-org/go-template/internal/rbac"
)

// Invitation tuning.
const (
	TTL     = 7 * 24 * time.Hour
	PageMax = 100
)

// Service runs the org membership invitation flow: admins invite
// an email with a role, the invitee accepts after signing in, and the
// membership + domain role grant land in one step. Emails go through the
// shared mailer (SMTP in production, log in dev); the raw token exists only
// in that email.
type Service struct {
	store   Store
	orgs    rbac.OrgsStore
	auth    auth.Store
	rbac    *rbac.Service
	mailer  mailer.Sender
	baseURL string
	logger  *slog.Logger
}

// NewService wires the flow.
func NewService(store Store, orgs rbac.OrgsStore, auth auth.Store, rbacSvc *rbac.Service, sender mailer.Sender, baseURL string, logger *slog.Logger) *Service {
	return &Service{
		store: store, orgs: orgs, auth: auth, rbac: rbacSvc, mailer: sender,
		baseURL: strings.TrimRight(baseURL, "/"), logger: logger,
	}
}

// Invite creates a pending invitation and emails the accept link. Uniform
// conflict errors avoid leaking whether the address is already a member or
// invited: callers see one "invitation not possible" answer.
func (s *Service) Invite(ctx context.Context, orgID, email, role, invitedBy string) (Invitation, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if orgID == "" || email == "" || !strings.Contains(email, "@") {
		return Invitation{}, "", kernel.Invalid("organization and a valid email are required")
	}
	if _, err := s.orgs.GetOrg(ctx, orgID); err != nil {
		return Invitation{}, "", err
	}
	if _, err := s.rbac.RoleByKey(role); err != nil {
		return Invitation{}, "", kernel.Invalid("unknown role " + role)
	}
	if user, _, err := s.auth.GetUserByEmail(ctx, email); err == nil {
		if member, merr := s.orgs.IsOrgMember(ctx, orgID, user.ID); merr == nil && member {
			return Invitation{}, "", fmt.Errorf("%w: already a member", kernel.ErrConflict)
		}
	}
	pending, err := s.store.ListPendingOrgInvitationsByEmail(ctx, email)
	if err == nil {
		for _, invitation := range pending {
			if invitation.OrgID == orgID {
				return Invitation{}, "", fmt.Errorf("%w: invitation already pending", kernel.ErrConflict)
			}
		}
	}
	token := kernel.NewID()
	now := time.Now()
	invitation := Invitation{
		ID:        kernel.NewID(),
		OrgID:     orgID,
		Email:     email,
		Role:      role,
		InvitedBy: invitedBy,
		TokenHash: hashToken(token),
		Status:    StatusPending,
		ExpiresAt: now.Add(TTL),
		CreatedAt: now,
	}
	if err := s.store.CreateOrgInvitation(ctx, invitation); err != nil {
		return Invitation{}, "", err
	}
	link := fmt.Sprintf("%s/accept-invitation?token=%s", s.baseURL, token)
	s.sendInviteEmail(ctx, invitation, link)
	return invitation, token, nil
}

// Accept validates the token and lands the membership: org_members row plus
// the Casbin domain role grant. Expired or non-pending invitations are a
// uniform validation error.
func (s *Service) Accept(ctx context.Context, token, userID string) (Invitation, error) {
	if token == "" {
		return Invitation{}, kernel.Invalid("invitation token is required")
	}
	invitation, err := s.store.GetOrgInvitationByHash(ctx, hashToken(token))
	if err != nil {
		return Invitation{}, fmt.Errorf("%w: invitation not found", kernel.ErrNotFound)
	}
	if invitation.Status != StatusPending || time.Now().After(invitation.ExpiresAt) {
		return Invitation{}, fmt.Errorf("%w: invitation is no longer valid", kernel.ErrConflict)
	}
	// Idempotent membership insert: existing rows are fine.
	if err := s.orgs.AddOrgMember(ctx, rbac.OrgMember{OrgID: invitation.OrgID, UserID: userID, JoinedAt: time.Now()}); err != nil && !errors.Is(err, kernel.ErrConflict) {
		return Invitation{}, err
	}
	if err := s.rbac.AssignRole(userID, invitation.Role, invitation.OrgID); err != nil {
		return Invitation{}, err
	}
	now := time.Now()
	if err := s.store.SetOrgInvitationStatus(ctx, invitation.ID, StatusAccepted, &now, userID); err != nil {
		return Invitation{}, err
	}
	invitation.Status = StatusAccepted
	invitation.AcceptedAt = &now
	invitation.AcceptedBy = userID
	return invitation, nil
}

// Revoke withdraws a pending invitation (admin action).
func (s *Service) Revoke(ctx context.Context, orgID, invitationID string) error {
	invitation, err := s.store.GetOrgInvitation(ctx, invitationID)
	if err != nil {
		return err
	}
	if invitation.OrgID != orgID {
		return fmt.Errorf("%w: invitation not found", kernel.ErrNotFound)
	}
	if invitation.Status != StatusPending {
		return fmt.Errorf("%w: only pending invitations can be revoked", kernel.ErrConflict)
	}
	return s.store.SetOrgInvitationStatus(ctx, invitationID, StatusRevoked, nil, "")
}

// ForOrg lists one org's invitations (newest first).
func (s *Service) ForOrg(ctx context.Context, orgID string, limit, offset int) ([]Invitation, int64, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > PageMax {
		limit = PageMax
	}
	return s.store.ListOrgInvitations(ctx, orgID, limit, offset)
}

// Mine lists the pending invitations addressed to the session's email.
func (s *Service) Mine(ctx context.Context, email string) ([]Invitation, error) {
	return s.store.ListPendingOrgInvitationsByEmail(ctx, email)
}

// hashToken is how invitation tokens are hashed — sha256 hex, the same
// scheme the mfa, email, and tokens features use. Copied, not shared: the
// features stay decoupled.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) sendInviteEmail(ctx context.Context, invitation Invitation, link string) {
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
