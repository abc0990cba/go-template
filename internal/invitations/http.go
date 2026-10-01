package invitations

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/users"
)

// HTTP carries the invitation handlers' dependencies.
type HTTP struct {
	Svc    *Service
	Audit  *audit.Service
	Logger *slog.Logger
	// RequirePermission is the assembly's catalog-permission guard.
	RequirePermission func(obj, act string) gin.HandlerFunc
}

// RegisterHTTP mounts the invitation routes on the authorized group: the
// org-admin routes (invite/list/revoke, orgs:read / orgs:update) and the
// self-service routes (mine / accept — auth only).
func RegisterHTTP(authorized *gin.RouterGroup, h HTTP) {
	authorized.GET("/orgs/:id/invitations", h.RequirePermission("orgs", "read"), h.orgInvitations)
	authorized.POST("/orgs/:id/invitations", h.RequirePermission("orgs", "update"), h.inviteOrgMember)
	authorized.DELETE("/orgs/:id/invitations/:invId", h.RequirePermission("orgs", "update"), h.revokeOrgInvitation)
	authorized.GET("/invitations", h.mine)
	authorized.POST("/invitations/accept", h.accept)
}

// inviteOrgMember creates an invitation (orgs:update). Audited; the raw
// token is returned so tests/devtools can complete the flow without reading
// the mail log.
func (h *HTTP) inviteOrgMember(c *gin.Context) {
	orgID := c.Param("id")
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	user := users.CurrentUser(c)
	invitation, token, err := h.Svc.Invite(c.Request.Context(), orgID, req.Email, req.Role, user.Email)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "org.invited", "org", orgID, nil, gin.H{"email": invitation.Email, "role": invitation.Role})
	c.JSON(http.StatusCreated, gin.H{
		"id":         invitation.ID,
		"org_id":     invitation.OrgID,
		"email":      invitation.Email,
		"role":       invitation.Role,
		"expires_at": invitation.ExpiresAt,
		// Dev convenience: production clients complete the flow from the
		// email link; the token is single-use and expires in 7 days.
		"token": token,
	})
}

// orgInvitations lists one org's invitations (orgs:read).
func (h *HTTP) orgInvitations(c *gin.Context) {
	orgID := c.Param("id")
	limit := httpx.IntQuery(c, "limit", 25)
	offset := httpx.IntQuery(c, "offset", 0)
	invitations, total, err := h.Svc.ForOrg(c.Request.Context(), orgID, limit, offset)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(invitations), "total": total, "limit": limit, "offset": offset})
}

// revokeOrgInvitation withdraws a pending invitation (orgs:update).
func (h *HTTP) revokeOrgInvitation(c *gin.Context) {
	orgID, invitationID := c.Param("id"), c.Param("invId")
	if err := h.Svc.Revoke(c.Request.Context(), orgID, invitationID); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "org.invitation_revoked", "org", orgID, nil, gin.H{"invitation": invitationID})
	c.Status(http.StatusNoContent)
}

// mine lists pending invitations addressed to the caller's email.
func (h *HTTP) mine(c *gin.Context) {
	user := users.CurrentUser(c)
	invitations, err := h.Svc.Mine(c.Request.Context(), user.Email)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": httpx.ListOrEmpty(invitations)})
}

// accept completes an invitation for the signed-in caller.
func (h *HTTP) accept(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	user := users.CurrentUser(c)
	invitation, err := h.Svc.Accept(c.Request.Context(), req.Token, user.ID)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "org.invitation_accepted", "org", invitation.OrgID, nil, gin.H{"role": invitation.Role, "email": invitation.Email})
	c.JSON(http.StatusOK, gin.H{"org_id": invitation.OrgID, "role": invitation.Role})
}
