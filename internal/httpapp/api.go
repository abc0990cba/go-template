package httpapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (a *App) apiRegister(c *gin.Context) {
	var req struct{ Email, Name, Password string }
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	user, token, err := a.services.Auth.Register(c.Request.Context(), req.Email, req.Name, req.Password)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": user, "token": token})
}

func (a *App) apiLogin(c *gin.Context) {
	var req struct{ Email, Password string }
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	user, token, err := a.services.Auth.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user, "token": token})
}

func (a *App) apiLogout(c *gin.Context) {
	// A missing session is the normal case for logout; nothing to report.
	_ = a.services.Auth.Logout(c.Request.Context(), a.sessionToken(c))
	c.Status(http.StatusNoContent)
}

func (a *App) apiMe(c *gin.Context) {
	user, err := a.services.Auth.CurrentUser(c.Request.Context(), a.sessionToken(c))
	if err != nil {
		a.problem(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (a *App) apiUsers(c *gin.Context) {
	users, next, err := a.services.Users.List(c.Request.Context(), listOptions(c))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": users, "next_cursor": next})
}

func (a *App) apiCreateUser(c *gin.Context) {
	var req struct{ Email, Name string }
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	user, err := a.services.Users.Create(c.Request.Context(), req.Email, req.Name)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, user)
}

func (a *App) apiAnalytics(c *gin.Context) {
	c.JSON(http.StatusOK, a.services.Analytics.Summary(c.Request.Context()))
}
