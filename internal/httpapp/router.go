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
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/platform/oauth"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

// App carries the handler dependencies; all handlers are methods on it.
type App struct {
	cfg      config.Config
	services *service.Container
	logger   *slog.Logger
	pinger   service.Pinger
	oauth    *oauth.Manager
	limits   *ipBuckets
}

// NewRouter builds the complete Gin engine: middleware, health endpoints,
// OpenAPI docs, HTML UI routes, and the /api group.
func NewRouter(cfg config.Config, services *service.Container, logger *slog.Logger, pinger service.Pinger, build observ.Build) http.Handler {
	app := &App{cfg: cfg, services: services, logger: logger, pinger: pinger, oauth: oauth.NewManager(cfg), limits: newIPBuckets(cfg.AuthRateRPS, cfg.AuthRateBurst)}
	r := gin.New()
	r.Use(gin.Recovery(), app.securityHeaders(), app.csrfIssue(), app.requestID(), app.metrics(), app.accessLog(), app.bodyLimit())
	r.NoRoute(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		app.problem(c, http.StatusNotFound, "not found", nil)
	})
	r.GET("/health/live", app.ok)
	r.GET("/health/ready", app.ready)
	r.GET("/health/startup", app.ok)
	r.GET("/metrics", metricsHandler())
	r.GET("/version", versionHandler(build))
	if cfg.PprofEnabled {
		r.GET("/debug/pprof/*any", pprofHandler())
	}
	r.GET("/openapi.yaml", app.openapi)
	r.GET("/docs", app.docs)

	r.GET("/", app.dashboard)
	r.GET("/login", app.loginPage)
	r.POST("/login", app.authRateLimit(), app.publicForm(app.login))
	r.GET("/register", app.registerPage)
	r.POST("/register", app.authRateLimit(), app.publicForm(app.register))
	r.POST("/logout", app.form(app.logout))
	r.GET("/profile", app.requireUser(app.profile))
	r.POST("/profile/logout-all", app.form(app.logoutAll))
	r.POST("/profile/unlink/:provider", app.form(app.unlinkProvider))
	r.GET("/auth/:provider/login", app.requireUser(app.oauthLogin))
	r.GET("/auth/:provider/callback", app.oauthCallback)
	r.GET("/users", app.requireUser(app.usersPage))
	r.POST("/users", app.form(app.createUser))
	r.POST("/users/:id", app.form(app.updateUser))
	r.POST("/users/:id/delete", app.form(app.deleteUser))
	r.GET("/enrichment", app.requireUser(app.enrichmentPage))
	r.POST("/enrichment", app.form(app.enqueueEnrichment))
	r.GET("/jobs", app.requireUser(app.jobsPage))
	r.POST("/jobs/process", app.form(app.processJob))
	r.GET("/reports", app.requireUser(app.reportsPage))
	r.POST("/reports", app.form(app.generateReport))
	r.GET("/analytics", app.requireUser(app.analyticsPage))

	api := r.Group("/api")
	api.POST("/auth/register", app.authRateLimit(), app.apiRegister)
	api.POST("/auth/login", app.authRateLimit(), app.apiLogin)
	api.POST("/auth/logout", app.apiLogout)
	api.GET("/me", app.apiMe)
	api.GET("/users", app.apiUsers)
	api.POST("/users", app.apiCreateUser)
	api.GET("/users/:id", app.apiGetUser)
	api.PUT("/users/:id", app.apiUpdateUser)
	api.DELETE("/users/:id", app.apiDeleteUser)
	api.GET("/jobs", app.apiJobs)
	api.POST("/jobs/enrichment", app.apiEnrichEnrichment)
	api.GET("/reports", app.apiReports)
	api.POST("/reports", app.apiGenerateReport)
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

// form guards a state-changing HTML action: active session plus CSRF.
func (a *App) form(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := a.services.Auth.CurrentUser(c.Request.Context(), a.sessionToken(c))
		if err != nil {
			c.Redirect(http.StatusSeeOther, "/login")
			return
		}
		c.Set("user_id", user.ID)
		if !a.verifyCSRF(c) {
			c.Abort()
			return
		}
		next(c)
	}
}

// publicForm guards an anonymous HTML action (login, register) with CSRF.
func (a *App) publicForm(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.verifyCSRF(c) {
			c.Abort()
			return
		}
		next(c)
	}
}

// setSession issues the session cookie with the hardening flags; the TTL
// itself is enforced server-side.
func (a *App) setSession(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("app_session", token, int(a.cfg.SessionTTL.Seconds()), "/", "", a.cfg.CookieSecure, true)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("app_csrf", newRandomToken(), int(a.cfg.SessionTTL.Seconds()), "/", "", a.cfg.CookieSecure, false)
}

func (a *App) clearSession(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("app_session", "", -1, "/", "", a.cfg.CookieSecure, true)
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
