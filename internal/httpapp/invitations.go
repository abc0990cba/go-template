package httpapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// apiInviteOrgMember creates an invitation (orgs:update). Audited; the raw
// token is returned so tests/devtools can complete the flow without reading
// the mail log.
func (a *App) apiInviteOrgMember(c *gin.Context) {
	orgID := c.Param("id")
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !a.bind(c, &req) {
		return
	}
	user := a.currentUser(c)
	invitation, token, err := a.services.Invitations.Invite(c.Request.Context(), orgID, req.Email, req.Role, user.Email)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "org.invited", "org", orgID, nil, gin.H{"email": invitation.Email, "role": invitation.Role})
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

// apiOrgInvitations lists one org's invitations (orgs:read).
func (a *App) apiOrgInvitations(c *gin.Context) {
	orgID := c.Param("id")
	limit := intQuery(c, "limit", 25)
	offset := intQuery(c, "offset", 0)
	invitations, total, err := a.services.Invitations.ForOrg(c.Request.Context(), orgID, limit, offset)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(invitations), "total": total, "limit": limit, "offset": offset})
}

// apiRevokeOrgInvitation withdraws a pending invitation (orgs:update).
func (a *App) apiRevokeOrgInvitation(c *gin.Context) {
	orgID, invitationID := c.Param("id"), c.Param("invId")
	if err := a.services.Invitations.Revoke(c.Request.Context(), orgID, invitationID); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "org.invitation_revoked", "org", orgID, nil, gin.H{"invitation": invitationID})
	c.Status(http.StatusNoContent)
}

// apiMyInvitations lists pending invitations addressed to the caller's email.
func (a *App) apiMyInvitations(c *gin.Context) {
	user := a.currentUser(c)
	invitations, err := a.services.Invitations.Mine(c.Request.Context(), user.Email)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": listOrEmpty(invitations)})
}

// apiAcceptInvitation completes an invitation for the signed-in caller.
func (a *App) apiAcceptInvitation(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}
	if !a.bind(c, &req) {
		return
	}
	user := a.currentUser(c)
	invitation, err := a.services.Invitations.Accept(c.Request.Context(), req.Token, user.ID)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "org.invitation_accepted", "org", invitation.OrgID, nil, gin.H{"role": invitation.Role, "email": invitation.Email})
	c.JSON(http.StatusOK, gin.H{"org_id": invitation.OrgID, "role": invitation.Role})
}
