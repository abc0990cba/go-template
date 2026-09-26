package httpapp

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/oapi-codegen/runtime/types"
	api "github.com/your-org/go-template/internal/api"
)

// derefString normalizes the generated optional fields; Email is a string
// alias so the same helper covers both after conversion at call sites.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return string(*s)
}

func derefEmail(e *types.Email) string {
	return derefString((*string)(e))
}

func (a *App) apiRegister(c *gin.Context) {
	var req api.RegisterRequest
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	user, token, err := a.services.Auth.Register(c.Request.Context(), string(req.Email), derefString(req.Name), req.Password)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": user, "token": token})
}

func (a *App) apiLogin(c *gin.Context) {
	var req api.LoginRequest
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	user, token, err := a.services.Auth.Login(c.Request.Context(), string(req.Email), req.Password)
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
	var req api.CreateUserRequest
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	user, err := a.services.Users.Create(c.Request.Context(), derefEmail(&req.Email), derefString(req.Name))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, user)
}

func (a *App) apiGetUser(c *gin.Context) {
	user, err := a.services.Users.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (a *App) apiUpdateUser(c *gin.Context) {
	var req api.UpdateUserRequest
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	user, err := a.services.Users.Update(c.Request.Context(), c.Param("id"), derefEmail(&req.Email), req.Name)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (a *App) apiDeleteUser(c *gin.Context) {
	if err := a.services.Users.Delete(c.Request.Context(), c.Param("id")); err != nil {
		a.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (a *App) apiJobs(c *gin.Context) {
	jobs, err := a.services.Jobs.List(c.Request.Context())
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": jobs})
}

func (a *App) apiEnrichEnrichment(c *gin.Context) {
	var req api.EnqueueEnrichmentJSONBody
	if err := c.BindJSON(&req); err != nil {
		a.problem(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	payload, err := json.Marshal(struct {
		Count int `json:"count"`
	}{Count: req.Count})
	if err != nil {
		a.fail(c, err)
		return
	}
	job, err := a.services.Jobs.Enqueue(c.Request.Context(), "user.enrich_batch", string(payload))
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}

func (a *App) apiGenerateReport(c *gin.Context) {
	job, err := a.services.Jobs.Enqueue(c.Request.Context(), "user.report_generate", "{}")
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}

func (a *App) apiReports(c *gin.Context) {
	reports, err := a.services.Reports.List(c.Request.Context())
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": reports})
}

func (a *App) apiAnalytics(c *gin.Context) {
	c.JSON(http.StatusOK, a.services.Analytics.Summary(c.Request.Context()))
}
