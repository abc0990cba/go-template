package httpapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// mfaStatus answers the profile page's two-factor section.
func (a *App) apiMFAStatus(c *gin.Context) {
	user := a.currentUser(c)
	enabled, pending, err := a.services.MFA.Status(c.Request.Context(), user.ID)
	if err != nil {
		a.fail(c, err)
		return
	}
	required := a.services.Settings.Bool(c.Request.Context(), "mfa.required")
	c.JSON(http.StatusOK, gin.H{"enabled": enabled, "pending": pending, "required": required})
}

// apiMFAEnroll starts TOTP enrollment: it returns the base32 secret and the
// otpauth:// URI the SPA renders as a QR code.
func (a *App) apiMFAEnroll(c *gin.Context) {
	user := a.currentUser(c)
	secret, uri, err := a.services.MFA.Enroll(c.Request.Context(), user.ID)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "mfa.enroll_started", "user", user.ID, nil, nil)
	c.JSON(http.StatusOK, gin.H{"secret": secret, "otpauth_url": uri})
}

// apiMFAVerify confirms enrollment with a code from the authenticator and
// returns the recovery codes — the only time they are ever shown.
func (a *App) apiMFAVerify(c *gin.Context) {
	user := a.currentUser(c)
	var req struct {
		Code string `json:"code"`
	}
	if !a.bind(c, &req) {
		return
	}
	codes, err := a.services.MFA.VerifyEnrollment(c.Request.Context(), user.ID, req.Code)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "mfa.enabled", "user", user.ID, nil, nil)
	c.JSON(http.StatusOK, gin.H{"recovery_codes": codes})
}

// apiMFADisable turns two-factor off; password plus a valid code required.
func (a *App) apiMFADisable(c *gin.Context) {
	user := a.currentUser(c)
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !a.bind(c, &req) {
		return
	}
	if err := a.services.MFA.Disable(c.Request.Context(), user.ID, req.Password, req.Code); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "mfa.disabled", "user", user.ID, nil, nil)
	c.Status(http.StatusNoContent)
}

// apiMFAChallenge completes the second login step: challenge token plus a
// TOTP or recovery code. Success behaves like a regular login (cookie +
// token). Guarded by the same tight auth rate budget as login.
func (a *App) apiMFAChallenge(c *gin.Context) {
	var req struct {
		Challenge string `json:"challenge"`
		Code      string `json:"code"`
	}
	if !a.bind(c, &req) {
		return
	}
	user, token, err := a.services.MFA.CompleteChallenge(c.Request.Context(), req.Challenge, req.Code)
	if err != nil {
		a.fail(c, err)
		return
	}
	a.setSession(c, token)
	a.audit(c, "user.logged_in_mfa", "user", user.ID, nil, nil)
	c.JSON(http.StatusOK, gin.H{"user": user, "token": token})
}

// apiMFAReset wipes a user's factor and recovery codes on admin order.
func (a *App) apiMFAReset(c *gin.Context) {
	target := c.Param("id")
	if err := a.services.MFA.Reset(c.Request.Context(), target); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "mfa.reset", "user", target, nil, nil)
	if err := a.services.Notifications.Create(c.Request.Context(), target, "mfa.reset", "Two-factor authentication reset",
		"An administrator reset your two-factor authentication. Set it up again from your profile.", "/profile"); err != nil {
		a.logger.Warn("mfa reset notification failed", "err", err, "user_id", target)
	}
	c.Status(http.StatusNoContent)
}
