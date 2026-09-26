package httpapp

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/model"
)

// fmtInt renders counters inside templ attributes.
func fmtInt(n int) string { return strconv.Itoa(n) }

// render writes a templ component. Generation:
//
//	go run github.com/a-h/templ/cmd/templ@latest generate
func (a *App) render(c *gin.Context, component templ.Component) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(c.Request.Context(), c.Writer); err != nil {
		a.logger.Error("template render failed", "err", err)
	}
}

// csrfToken returns the token issued for the double-submit check.
func (a *App) csrfToken(c *gin.Context) string { return c.GetString("csrf_token") }

// renderError shows a client-safe message, or a generic one after logging the
// real error, so UI handlers never fail silently.
func (a *App) renderError(c *gin.Context, page string, err error) {
	msg, safe := clientSafeMessage(err)
	if !safe {
		a.logger.Error("page action failed", "page", page, "err", err)
		msg = "something went wrong, try again"
	}
	switch page {
	case "login":
		a.render(c, loginPage(msg, a.csrfToken(c)))
	case "register":
		a.render(c, registerPage(msg, a.csrfToken(c)))
	case "profile":
		a.renderProfileData(c, msg)
	case "users":
		a.renderUsersData(c, msg)
	case "enrichment":
		a.renderJobsData(c, msg)
	case "reports":
		a.renderReportsData(c, msg)
	}
}

func (a *App) currentUser(c *gin.Context) (model.User, bool) {
	user, err := a.services.Auth.CurrentUser(c.Request.Context(), a.sessionToken(c))
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/login")
		return model.User{}, false
	}
	return user, true
}

func (a *App) renderProfileData(c *gin.Context, errText string) {
	user, ok := a.currentUser(c)
	if !ok {
		return
	}
	identities, err := a.services.Auth.Identities(c.Request.Context(), user.ID)
	if err != nil {
		a.logger.Error("list identities failed", "err", err)
	}
	a.render(c, profilePage(user, identities, errText, a.csrfToken(c)))
}

func (a *App) dashboard(c *gin.Context) {
	if _, ok := a.currentUser(c); !ok {
		return
	}
	summary := a.services.Analytics.Summary(c.Request.Context())
	a.render(c, dashboardPage(summary, a.csrfToken(c)))
}

func (a *App) loginPage(c *gin.Context)    { a.render(c, loginPage("", a.csrfToken(c))) }
func (a *App) registerPage(c *gin.Context) { a.render(c, registerPage("", a.csrfToken(c))) }

func (a *App) register(c *gin.Context) {
	_, token, err := a.services.Auth.Register(c.Request.Context(), c.PostForm("email"), c.PostForm("name"), c.PostForm("password"))
	if err != nil {
		a.renderError(c, "register", err)
		return
	}
	a.setSession(c, token)
	c.Redirect(http.StatusSeeOther, "/")
}

func (a *App) login(c *gin.Context) {
	_, token, err := a.services.Auth.Login(c.Request.Context(), c.PostForm("email"), c.PostForm("password"))
	if err != nil {
		a.renderError(c, "login", err)
		return
	}
	a.setSession(c, token)
	c.Redirect(http.StatusSeeOther, "/")
}

func (a *App) logout(c *gin.Context) {
	// A missing session is the normal case for logout; nothing to report.
	_ = a.services.Auth.Logout(c.Request.Context(), a.sessionToken(c))
	a.clearSession(c)
	c.Redirect(http.StatusSeeOther, "/login")
}

func (a *App) profile(c *gin.Context) {
	a.renderProfileData(c, "")
}

// logoutAll revokes every session of the current user.
func (a *App) logoutAll(c *gin.Context) {
	user, ok := a.currentUser(c)
	if !ok {
		return
	}
	if err := a.services.Auth.LogoutAll(c.Request.Context(), user.ID); err != nil {
		a.renderError(c, "profile", err)
		return
	}
	a.clearSession(c)
	c.Redirect(http.StatusSeeOther, "/login")
}

func (a *App) unlinkProvider(c *gin.Context) {
	user, ok := a.currentUser(c)
	if !ok {
		return
	}
	if err := a.services.Auth.UnlinkProvider(c.Request.Context(), user.ID, c.Param("provider")); err != nil {
		a.renderError(c, "profile", err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/profile")
}

func (a *App) renderUsersData(c *gin.Context, errText string) {
	users, next, err := a.services.Users.List(c.Request.Context(), listOptions(c))
	if err != nil {
		a.logger.Error("list users failed", "err", err)
	}
	a.render(c, usersPage(users, next, c.DefaultQuery("mode", "offset"), errText, a.csrfToken(c)))
}

func (a *App) usersPage(c *gin.Context) {
	a.renderUsersData(c, "")
}

func (a *App) createUser(c *gin.Context) {
	if _, err := a.services.Users.Create(c.Request.Context(), c.PostForm("email"), c.PostForm("name")); err != nil {
		a.renderError(c, "users", err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/users")
}

func (a *App) updateUser(c *gin.Context) {
	if _, err := a.services.Users.Update(c.Request.Context(), c.Param("id"), c.PostForm("email"), c.PostForm("name")); err != nil {
		a.renderError(c, "users", err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/users")
}

func (a *App) deleteUser(c *gin.Context) {
	if err := a.services.Users.Delete(c.Request.Context(), c.Param("id")); err != nil {
		a.renderError(c, "users", err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/users")
}

func (a *App) renderJobsData(c *gin.Context, errText string) {
	jobs, err := a.services.Jobs.List(c.Request.Context())
	if err != nil {
		a.logger.Error("list jobs failed", "err", err)
	}
	a.render(c, enrichmentPage(jobs, errText, a.csrfToken(c)))
}

func (a *App) enrichmentPage(c *gin.Context) {
	a.renderJobsData(c, "")
}

func (a *App) enqueueEnrichment(c *gin.Context) {
	count, err := strconv.Atoi(c.DefaultPostForm("count", "10"))
	if err != nil || count <= 0 || count > 100 {
		a.renderJobsData(c, "count must be a number between 1 and 100")
		return
	}
	payload, err := json.Marshal(struct {
		Count int `json:"count"`
	}{Count: count})
	if err != nil {
		a.logger.Error("marshal enrichment payload failed", "err", err)
		a.renderJobsData(c, "something went wrong, try again")
		return
	}
	if _, err := a.services.Jobs.Enqueue(c.Request.Context(), "user.enrich_batch", string(payload)); err != nil {
		a.renderError(c, "enrichment", err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/enrichment")
}

func (a *App) jobsPage(c *gin.Context) {
	jobs, err := a.services.Jobs.List(c.Request.Context())
	if err != nil {
		a.logger.Error("list jobs failed", "err", err)
	}
	a.render(c, jobsPage(jobs, "", a.csrfToken(c)))
}

func (a *App) processJob(c *gin.Context) {
	if err := a.services.Jobs.ProcessOne(c.Request.Context()); err != nil {
		a.logger.Error("job processing failed", "err", err)
	}
	c.Redirect(http.StatusSeeOther, "/jobs")
}

func (a *App) renderReportsData(c *gin.Context, errText string) {
	reports, err := a.services.Reports.List(c.Request.Context())
	if err != nil {
		a.logger.Error("list reports failed", "err", err)
	}
	a.render(c, reportsPage(reports, errText, a.csrfToken(c)))
}

func (a *App) reportsPage(c *gin.Context) {
	a.renderReportsData(c, "")
}

func (a *App) generateReport(c *gin.Context) {
	if _, err := a.services.Reports.Generate(c.Request.Context()); err != nil {
		a.renderError(c, "reports", err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/reports")
}

func (a *App) analyticsPage(c *gin.Context) {
	summary := a.services.Analytics.Summary(c.Request.Context())
	a.render(c, analyticsPage(summary, a.csrfToken(c)))
}

func (a *App) openapi(c *gin.Context) {
	c.File("api/openapi/openapi.yaml")
}

func (a *App) docs(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, `<!doctype html><html><head><title>API Docs</title><script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script></head><body><script id="api-reference" data-url="/openapi.yaml"></script></body></html>`)
}
