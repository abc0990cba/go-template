// Email-flow HTTP endpoints. All three answer uniformly so they cannot be
// used to probe which emails exist.
package email

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/settings"
)

// HTTP carries the email-flow handlers' dependencies.
type HTTP struct {
	Svc      *Service
	Settings *settings.Service
	Audit    *audit.Service
	// AuthRateLimit is the assembly's tighter per-IP budget for credential
	// endpoints.
	AuthRateLimit gin.HandlerFunc
}

// RegisterHTTP mounts the password-reset and verification flows on the
// public group.
func RegisterHTTP(public *gin.RouterGroup, h HTTP) {
	public.POST("/auth/forgot-password", h.AuthRateLimit, h.forgotPassword)
	public.POST("/auth/reset-password", h.AuthRateLimit, h.resetPassword)
	public.POST("/auth/verify-email", h.verifyEmail)
}

func (h *HTTP) forgotPassword(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" {
		httpx.Problem(c, http.StatusBadRequest, "invalid request", nil)
		return
	}
	if !h.Settings.Bool(c.Request.Context(), "password_reset.enabled") {
		c.Status(http.StatusNoContent)
		return
	}
	if err := h.Svc.RequestPasswordReset(c.Request.Context(), req.Email); err != nil {
		// Same uniform answer as success: no account existence leak.
		_ = err
	}
	c.Status(http.StatusNoContent)
}

func (h *HTTP) resetPassword(c *gin.Context) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	if err := h.Svc.ResetPassword(c.Request.Context(), req.Token, req.Password); err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	audit.Record(h.Audit, c, "user.password_reset", "user", "", nil, nil)
	c.Status(http.StatusNoContent)
}

func (h *HTTP) verifyEmail(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	if err := h.Svc.VerifyEmail(c.Request.Context(), req.Token); err != nil {
		httpx.Fail(c, nil, err)
		return
	}
	c.Status(http.StatusNoContent)
}
