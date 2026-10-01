// MFA HTTP surface: the challenge endpoint completes the second login step
// on the public group under the tight auth rate budget; enrollment,
// verification, and disable are profile self-service on the authorized
// group; the admin reset lives on the users resource.

package mfa

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/notifications"
	"github.com/your-org/go-template/internal/users"
)

// HTTP carries the MFA handlers' dependencies.
type HTTP struct {
	Svc           *Service
	Settings      Settings
	Notifications *notifications.Service
	Audit         *audit.Service
	Logger        *slog.Logger
	// AuthRateLimit is the assembly's tighter per-IP budget for the
	// challenge endpoint.
	AuthRateLimit gin.HandlerFunc
	// RequirePermission is the assembly's catalog-permission guard.
	RequirePermission func(obj, act string) gin.HandlerFunc
	// SessionTTL and CookieSecure mirror the login cookie flags for the
	// session the completed challenge opens.
	SessionTTL   time.Duration
	CookieSecure bool
}

// RegisterHTTP mounts the two-factor routes on the public and authorized
// groups.
func RegisterHTTP(public, authorized *gin.RouterGroup, h HTTP) {
	public.POST("/auth/mfa/challenge", h.AuthRateLimit, h.challenge)
	authorized.GET("/auth/mfa/status", h.status)
	authorized.POST("/auth/mfa/enroll", h.enroll)
	authorized.POST("/auth/mfa/verify", h.verify)
	authorized.POST("/auth/mfa/disable", h.disable)
	authorized.DELETE("/users/:id/mfa", h.RequirePermission("users", "update"), h.reset)
}

// status answers the profile page's two-factor section.
func (h *HTTP) status(c *gin.Context) {
	user := users.CurrentUser(c)
	enabled, pending, err := h.Svc.Status(c.Request.Context(), user.ID)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	required := h.Settings.Bool(c.Request.Context(), "mfa.required")
	c.JSON(http.StatusOK, gin.H{"enabled": enabled, "pending": pending, "required": required})
}

// enroll starts TOTP enrollment: it returns the base32 secret and the
// otpauth:// URI the SPA renders as a QR code.
func (h *HTTP) enroll(c *gin.Context) {
	user := users.CurrentUser(c)
	secret, uri, err := h.Svc.Enroll(c.Request.Context(), user.ID)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "mfa.enroll_started", "user", user.ID, nil, nil)
	c.JSON(http.StatusOK, gin.H{"secret": secret, "otpauth_url": uri})
}

// verify confirms enrollment with a code from the authenticator and
// returns the recovery codes — the only time they are ever shown.
func (h *HTTP) verify(c *gin.Context) {
	user := users.CurrentUser(c)
	var req struct {
		Code string `json:"code"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	codes, err := h.Svc.VerifyEnrollment(c.Request.Context(), user.ID, req.Code)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "mfa.enabled", "user", user.ID, nil, nil)
	c.JSON(http.StatusOK, gin.H{"recovery_codes": codes})
}

// disable turns two-factor off; password plus a valid code required.
func (h *HTTP) disable(c *gin.Context) {
	user := users.CurrentUser(c)
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	if err := h.Svc.Disable(c.Request.Context(), user.ID, req.Password, req.Code); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "mfa.disabled", "user", user.ID, nil, nil)
	c.Status(http.StatusNoContent)
}

// challenge completes the second login step: challenge token plus a TOTP or
// recovery code. Success behaves like a regular login (cookie + token).
func (h *HTTP) challenge(c *gin.Context) {
	var req struct {
		Challenge string `json:"challenge"`
		Code      string `json:"code"`
	}
	if !httpx.Bind(c, &req) {
		return
	}
	user, token, err := h.Svc.CompleteChallenge(c.Request.Context(), req.Challenge, req.Code)
	if err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	httpx.SetSession(c, token, h.SessionTTL, h.CookieSecure)
	audit.Record(h.Audit, c, "user.logged_in_mfa", "user", user.ID, nil, nil)
	c.JSON(http.StatusOK, gin.H{"user": user, "token": token})
}

// reset wipes a user's factor and recovery codes on admin order.
func (h *HTTP) reset(c *gin.Context) {
	target := c.Param("id")
	if err := h.Svc.Reset(c.Request.Context(), target); err != nil {
		httpx.Fail(c, h.Logger, err)
		return
	}
	audit.Record(h.Audit, c, "mfa.reset", "user", target, nil, nil)
	if err := h.Notifications.Create(c.Request.Context(), target, "mfa.reset", "Two-factor authentication reset",
		"An administrator reset your two-factor authentication. Set it up again from your profile.", "/profile"); err != nil {
		h.Logger.Warn("mfa reset notification failed", "err", err, "user_id", target)
	}
	c.Status(http.StatusNoContent)
}
