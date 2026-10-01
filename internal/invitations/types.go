// Package invitations runs the org membership invitation flow: admins
// invite an email with a role, the invitee accepts after signing in, and
// the membership + domain role grant land in one step. It serves both the
// org-admin routes (invite/list/revoke) and the self-service routes (mine /
// accept).
package invitations

import "time"

// Invitation is a pending org membership offer. The raw token lives only
// in the invitation email; the store keeps its sha256 hash.
type Invitation struct {
	ID         string     `json:"id"`
	OrgID      string     `json:"org_id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	InvitedBy  string     `json:"invited_by,omitempty"`
	TokenHash  string     `json:"-"`
	Status     string     `json:"status"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	AcceptedBy string     `json:"accepted_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Invitation statuses.
const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRevoked  = "revoked"
	StatusExpired  = "expired"
)
