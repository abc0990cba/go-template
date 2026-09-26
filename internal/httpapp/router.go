// Package httpapp contains the HTTP layer: routing, middleware, the JSON API
// handlers, and the server-rendered UI.
package httpapp

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

// App carries the handler dependencies; all handlers are methods on it.
type App struct {
	cfg      config.Config
	services *service.Container
	logger   *slog.Logger
	pinger   service.Pinger
}

// NewRouter builds the complete Gin engine: middleware, health endpoints,
// OpenAPI docs, HTML UI routes, and the /api group.
func NewRouter(cfg config.Config, services *service.Container, logger *slog.Logger, pinger service.Pinger) http.Handler {
	app := &App{cfg: cfg, services: services, logger: logger, pinger: pinger}
	r := gin.New()
	r.Use(gin.Recovery(), app.requestID(), app.bodyLimit())
	r.GET("/health/live", app.ok)
	r.GET("/health/ready", app.ready)
	r.GET("/health/startup", app.ok)
	r.GET("/openapi.yaml", app.openapi)
	r.GET("/docs", app.docs)

	r.GET("/", app.dashboard)
	r.GET("/login", app.loginPage)
	r.POST("/login", app.login)
	r.GET("/register", app.registerPage)
	r.POST("/register", app.register)
	r.POST("/logout", app.logout)
	r.GET("/profile", app.requireUser(app.profile))
	r.POST("/profile/link/:provider", app.requireUser(app.linkProvider))
	r.POST("/profile/unlink/:provider", app.requireUser(app.unlinkProvider))
	r.GET("/users", app.requireUser(app.usersPage))
	r.POST("/users", app.requireUser(app.createUser))
	r.POST("/users/:id", app.requireUser(app.updateUser))
	r.POST("/users/:id/delete", app.requireUser(app.deleteUser))
	r.GET("/enrichment", app.requireUser(app.enrichmentPage))
	r.POST("/enrichment", app.requireUser(app.enqueueEnrichment))
	r.GET("/jobs", app.requireUser(app.jobsPage))
	r.POST("/jobs/process", app.requireUser(app.processJob))
	r.GET("/reports", app.requireUser(app.reportsPage))
	r.POST("/reports", app.requireUser(app.generateReport))
	r.GET("/analytics", app.requireUser(app.analyticsPage))

	api := r.Group("/api")
	api.POST("/auth/register", app.apiRegister)
	api.POST("/auth/login", app.apiLogin)
	api.POST("/auth/logout", app.apiLogout)
	api.GET("/me", app.apiMe)
	api.GET("/users", app.apiUsers)
	api.POST("/users", app.apiCreateUser)
	api.GET("/analytics/summary", app.apiAnalytics)
	return r
}

func (a *App) ok(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ready reflects dependency health; a failing check yields 503.
func (a *App) ready(c *gin.Context) {
	if err := a.pinger.Ping(c.Request.Context()); err != nil {
		a.logger.Error("readiness check failed", "err", err)
		a.problem(c, http.StatusServiceUnavailable, "not ready", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (a *App) problem(c *gin.Context, status int, title string, err error) {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	c.JSON(status, gin.H{"type": "about:blank", "title": title, "status": status, "detail": detail})
}

func (a *App) sessionToken(c *gin.Context) string {
	cookie, _ := c.Cookie("app_session")
	if cookie != "" {
		return cookie
	}
	header := c.GetHeader("Authorization")
	if token, ok := strings.CutPrefix(header, "Bearer "); ok {
		return token
	}
	return header
}

func (a *App) requireUser(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := a.services.Auth.CurrentUser(c.Request.Context(), a.sessionToken(c))
		if err != nil {
			c.Redirect(http.StatusSeeOther, "/login")
			return
		}
		c.Set("user_id", user.ID)
		next(c)
	}
}

func (a *App) setSession(c *gin.Context, token string) {
	c.SetCookie("app_session", token, 3600*24*7, "/", "", false, true)
}

func intQuery(c *gin.Context, name string, fallback int) int {
	value, err := strconv.Atoi(c.DefaultQuery(name, ""))
	if err != nil {
		return fallback
	}
	return value
}

func listOptions(c *gin.Context) repository.ListUsersOptions {
	return repository.ListUsersOptions{
		Query: c.Query("q"), Mode: c.DefaultQuery("mode", "offset"), Limit: intQuery(c, "limit", 25), Offset: intQuery(c, "offset", 0), After: c.Query("after"),
	}
}
