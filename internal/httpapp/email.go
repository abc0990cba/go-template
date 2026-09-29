// Email-flow HTTP endpoints. All three answer uniformly so they cannot be
// used to probe which emails exist.
package httpapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (a *App) apiForgotPassword(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" {
		a.problem(c, http.StatusBadRequest, "invalid request", nil)
		return
	}
	if !a.services.Settings.Bool(c.Request.Context(), "password_reset.enabled") {
		c.Status(http.StatusNoContent)
		return
	}
	if err := a.services.Email.RequestPasswordReset(c.Request.Context(), req.Email); err != nil {
		a.logger.Error("password reset request failed", "err", err)
	}
	c.Status(http.StatusNoContent)
}

func (a *App) apiResetPassword(c *gin.Context) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !a.bind(c, &req) {
		return
	}
	if err := a.services.Email.ResetPassword(c.Request.Context(), req.Token, req.Password); err != nil {
		a.fail(c, err)
		return
	}
	a.audit(c, "user.password_reset", "user", "", nil, nil)
	c.Status(http.StatusNoContent)
}

func (a *App) apiVerifyEmail(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}
	if !a.bind(c, &req) {
		return
	}
	if err := a.services.Email.VerifyEmail(c.Request.Context(), req.Token); err != nil {
		a.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
