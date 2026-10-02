// Impersonation: an administrator opens a session as another user; every
// route below the guard is flagged, and stopping restores the admin.

package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/users"
)

// impersonate starts an impersonation session for the target user.
func (h *HTTP) impersonate(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	admin := users.CurrentUser(c)
	if admin.ID == req.UserID {
		httpx.Problem(c, http.StatusBadRequest, "cannot impersonate yourself", nil)
		return
	}
	token, err := h.Svc.Impersonate(c.Request.Context(), admin.ID, req.UserID)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	httpx.SetSession(c, token, h.SessionTTL, h.CookieSecure)
	audit.Record(h.Audit, c, "impersonation.started", "user", req.UserID, nil, gin.H{"admin_id": admin.ID})
	if err := h.Notifications.Create(c.Request.Context(), req.UserID, "impersonation.started", "An administrator signed in to your account", "Contact support if this was not expected.", "/profile"); err != nil {
		h.Logger.Error("impersonation notification failed", "err", err)
	}
	c.JSON(http.StatusOK, gin.H{"status": "impersonating", "user_id": req.UserID})
}

// stopImpersonation ends the impersonated session and restores the admin.
func (h *HTTP) stopImpersonation(c *gin.Context) {
	session, err := h.Svc.CurrentSession(c.Request.Context(), httpx.SessionToken(c))
	if err != nil || session.ImpersonatedBy == "" {
		httpx.Problem(c, http.StatusBadRequest, "not impersonating", nil)
		return
	}
	adminToken, err := h.Svc.StopImpersonation(c.Request.Context(), httpx.SessionToken(c))
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	httpx.SetSession(c, adminToken, h.SessionTTL, h.CookieSecure)
	audit.Record(h.Audit, c, "impersonation.stopped", "user", session.UserID, nil, gin.H{"admin_id": session.ImpersonatedBy})
	c.Status(http.StatusNoContent)
}
