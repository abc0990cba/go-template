package httpapp

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/model"
)

//go:embed views.html
var viewsHTML string

var views = template.Must(template.New("views").Parse(viewsHTML))

func (a *App) render(c *gin.Context, name string, data gin.H) {
	if data == nil {
		data = gin.H{}
	}
	data["CSRFToken"] = c.GetString("csrf_token")
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := views.ExecuteTemplate(c.Writer, name, data); err != nil {
		a.logger.Error("template render failed", "template", name, "err", err)
	}
}

// renderError shows a client-safe message, or a generic one after logging the
// real error, so UI handlers never fail silently.
func (a *App) renderError(c *gin.Context, page string, err error) {
	msg, safe := clientSafeMessage(err)
	if !safe {
		a.logger.Error("page action failed", "page", page, "err", err)
		msg = "something went wrong, try again"
	}
	switch page {
	case "profile":
		a.renderProfileData(c, gin.H{"Error": msg})
	case "users":
		a.renderUsersData(c, gin.H{"Error": msg})
	case "enrichment":
		a.renderJobsData(c, gin.H{"Error": msg})
	default:
		a.render(c, page, gin.H{"Error": msg})
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

func (a *App) renderProfileData(c *gin.Context, data gin.H) {
	user, ok := a.currentUser(c)
	if !ok {
		return
	}
	identities, err := a.services.Auth.Identities(c.Request.Context(), user.ID)
	if err != nil {
		a.logger.Error("list identities failed", "err", err)
	}
	data["User"] = user
	data["Identities"] = identities
	a.render(c, "profile", data)
}

func (a *App) dashboard(c *gin.Context) {
	if _, ok := a.currentUser(c); !ok {
		return
	}
	a.render(c, "dashboard", gin.H{"Analytics": a.services.Analytics.Summary(c.Request.Context())})
}

func (a *App) loginPage(c *gin.Context)    { a.render(c, "login", nil) }
func (a *App) registerPage(c *gin.Context) { a.render(c, "register", nil) }

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
	a.renderProfileData(c, gin.H{})
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

func (a *App) renderUsersData(c *gin.Context, data gin.H) {
	users, next, err := a.services.Users.List(c.Request.Context(), listOptions(c))
	if err != nil {
		a.logger.Error("list users failed", "err", err)
	}
	data["Users"] = users
	data["Next"] = next
	data["Mode"] = c.DefaultQuery("mode", "offset")
	a.render(c, "users", data)
}

func (a *App) usersPage(c *gin.Context) {
	a.renderUsersData(c, gin.H{})
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

func (a *App) renderJobsData(c *gin.Context, data gin.H) {
	jobs, err := a.services.Jobs.List(c.Request.Context())
	if err != nil {
		a.logger.Error("list jobs failed", "err", err)
	}
	data["Jobs"] = jobs
	a.render(c, "enrichment", data)
}

func (a *App) enrichmentPage(c *gin.Context) {
	a.renderJobsData(c, gin.H{})
}

func (a *App) enqueueEnrichment(c *gin.Context) {
	count, err := strconv.Atoi(c.DefaultPostForm("count", "10"))
	if err != nil || count <= 0 || count > 100 {
		a.renderJobsData(c, gin.H{"Error": "count must be a number between 1 and 100"})
		return
	}
	payload, err := json.Marshal(struct {
		Count int `json:"count"`
	}{Count: count})
	if err != nil {
		a.logger.Error("marshal enrichment payload failed", "err", err)
		a.renderJobsData(c, gin.H{"Error": "something went wrong, try again"})
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
	a.render(c, "jobs", gin.H{"Jobs": jobs})
}

func (a *App) processJob(c *gin.Context) {
	if err := a.services.Jobs.ProcessOne(c.Request.Context()); err != nil {
		a.logger.Error("job processing failed", "err", err)
	}
	c.Redirect(http.StatusSeeOther, "/jobs")
}

func (a *App) reportsPage(c *gin.Context) {
	reports, err := a.services.Reports.List(c.Request.Context())
	if err != nil {
		a.logger.Error("list reports failed", "err", err)
	}
	a.render(c, "reports", gin.H{"Reports": reports})
}

func (a *App) generateReport(c *gin.Context) {
	if _, err := a.services.Reports.Generate(c.Request.Context()); err != nil {
		a.renderError(c, "reports", err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/reports")
}

func (a *App) analyticsPage(c *gin.Context) {
	a.render(c, "analytics", gin.H{"Analytics": a.services.Analytics.Summary(c.Request.Context())})
}

func (a *App) openapi(c *gin.Context) {
	c.File("api/openapi/openapi.yaml")
}

func (a *App) docs(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, `<!doctype html><html><head><title>API Docs</title><script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script></head><body><script id="api-reference" data-url="/openapi.yaml"></script></body></html>`)
}
