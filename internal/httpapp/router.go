// Package httpapp contains the HTTP layer: routing, middleware, the JSON
// API handlers, the SSE stream, and the embedded SPA.
package httpapp

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/platform/oauth"
	"github.com/your-org/go-template/internal/realtime"
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
	hub      *realtime.Hub
	limits   *ipBuckets
}

// NewRouter builds the complete Gin engine: middleware, health endpoints,
// OpenAPI docs, the /api group with the SSE stream, and the embedded SPA.
func NewRouter(cfg config.Config, services *service.Container, logger *slog.Logger, pinger service.Pinger, build observ.Build, hub *realtime.Hub) http.Handler {
	app := &App{
		cfg: cfg, services: services, logger: logger, pinger: pinger,
		oauth: oauth.NewManager(cfg), hub: hub,
		limits: newIPBuckets(cfg.AuthRateRPS, cfg.AuthRateBurst),
	}
	r := gin.New()
	r.Use(gin.Recovery(), app.securityHeaders(), app.requestID(), app.metrics(), app.accessLog(), app.bodyLimit())
	r.NoRoute(app.notFound)
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

	// JSON API. Authentication uses the session cookie (browser) or a
	// Bearer token (non-browser clients); unsafe methods are additionally
	// checked against Origin/Sec-Fetch-Site, replacing the old form CSRF.
	api := r.Group("/api", app.originCheck())
	api.POST("/auth/register", app.authRateLimit(), app.apiRegister)
	api.POST("/auth/login", app.authRateLimit(), app.apiLogin)
	api.POST("/auth/logout", app.apiLogout)
	api.GET("/oauth/providers", app.apiOAuthProviders)
	api.GET("/settings/public", app.apiPublicSettings)
	api.POST("/auth/forgot-password", app.authRateLimit(), app.apiForgotPassword)
	api.POST("/auth/reset-password", app.authRateLimit(), app.apiResetPassword)
	api.POST("/auth/verify-email", app.apiVerifyEmail)

	// Authenticated routes resolve the caller, then the active Casbin
	// domain (X-Org-Id header, else the user's single org, else the
	// platform). Guarded routes additionally require a catalog permission;
	// self-service routes (me, profile, sessions, SSE) only need auth.
	authorized := api.Group("", app.apiAuthRequired, app.orgDomain)
	authorized.GET("/me", app.apiMe)
	authorized.GET("/users", app.requirePermission("users", "read"), app.apiUsers)
	authorized.POST("/users", app.requirePermission("users", "create"), app.apiCreateUser)
	authorized.GET("/users/:id", app.requirePermission("users", "read"), app.apiGetUser)
	authorized.PUT("/users/:id", app.requirePermission("users", "update"), app.apiUpdateUser)
	authorized.DELETE("/users/:id", app.requirePermission("users", "delete"), app.apiDeleteUser)
	authorized.GET("/jobs", app.requirePermission("jobs", "read"), app.apiJobs)
	authorized.GET("/jobs/:id", app.requirePermission("jobs", "read"), app.apiGetJob)
	authorized.POST("/jobs/process", app.requirePermission("jobs", "execute"), app.apiProcessJob)
	authorized.POST("/jobs/enrichment", app.requirePermission("jobs", "execute"), app.apiEnrichEnrichment)
	authorized.GET("/enrichment-profiles", app.requirePermission("enrichments", "read"), app.apiEnrichmentProfiles)
	authorized.GET("/reports", app.requirePermission("reports", "read"), app.apiReports)
	authorized.POST("/reports", app.requirePermission("reports", "create"), app.apiGenerateReport)
	authorized.GET("/reports/:id", app.requirePermission("reports", "read"), app.apiGetReport)
	authorized.GET("/analytics/summary", app.requirePermission("analytics", "read"), app.apiAnalytics)
	authorized.GET("/analytics/timeseries", app.requirePermission("analytics", "read"), app.apiTimeseries)
	authorized.GET("/analytics/events", app.requirePermission("analytics", "read"), app.apiRecentEvents)
	authorized.GET("/analytics/breakdown", app.requirePermission("analytics", "read"), app.apiAnalyticsBreakdown)
	authorized.GET("/analytics/dau", app.requirePermission("analytics", "read"), app.apiAnalyticsDAU)
	authorized.GET("/me/identities", app.apiIdentities)
	authorized.GET("/sessions", app.apiSessions)
	authorized.POST("/profile/logout-all", app.apiLogoutAll)
	authorized.POST("/profile/unlink/:provider", app.apiUnlinkProvider)
	authorized.GET("/events/stream", app.sseStream)
	authorized.GET("/notifications", app.apiNotifications)
	authorized.GET("/notifications/unread-count", app.apiUnreadCount)
	authorized.POST("/notifications/:id/read", app.apiMarkNotificationRead)
	authorized.POST("/notifications/read-all", app.apiMarkAllNotificationsRead)
	authorized.POST("/impersonation", app.requirePermission("impersonation", "execute"), app.apiImpersonate)
	authorized.POST("/impersonation/stop", app.apiStopImpersonation)
	authorized.GET("/audit", app.requirePermission("audit", "read"), app.apiAudit)
	authorized.GET("/settings", app.requirePermission("settings", "read"), app.apiSettings)
	authorized.PUT("/settings/:key", app.requirePermission("settings", "update"), app.apiUpdateSetting)
	authorized.GET("/api-keys", app.apiListAPIKeys)
	authorized.POST("/api-keys", app.apiCreateAPIKey)
	authorized.DELETE("/api-keys/:id", app.apiRevokeAPIKey)
	authorized.GET("/webhooks", app.requirePermission("webhooks", "read"), app.apiListWebhooks)
	authorized.POST("/webhooks", app.requirePermission("webhooks", "create"), app.apiCreateWebhook)
	authorized.PUT("/webhooks/:id", app.requirePermission("webhooks", "update"), app.apiUpdateWebhook)
	authorized.DELETE("/webhooks/:id", app.requirePermission("webhooks", "delete"), app.apiDeleteWebhook)
	authorized.GET("/webhooks/:id/deliveries", app.requirePermission("webhooks", "read"), app.apiWebhookDeliveries)
	authorized.POST("/webhooks/:id/test", app.requirePermission("webhooks", "execute"), app.apiTestWebhook)

	// Tickets: requesters file and track their own, approvers decide on
	// their queue. Fine-grained ownership and step-role checks live in the
	// service; these guards keep the catalog permissions.
	authorized.GET("/tickets", app.requirePermission("tickets", "read"), app.apiListTickets)
	authorized.GET("/tickets/counts", app.requirePermission("tickets", "read"), app.apiTicketCounts)
	authorized.POST("/tickets", app.requirePermission("tickets", "create"), app.apiCreateTicket)
	authorized.GET("/tickets/:id", app.requirePermission("tickets", "read"), app.apiGetTicket)
	authorized.PATCH("/tickets/:id", app.requirePermission("tickets", "update"), app.apiUpdateTicket)
	authorized.DELETE("/tickets/:id", app.requirePermission("tickets", "delete"), app.apiDeleteTicket)
	authorized.POST("/tickets/:id/actions", app.requirePermission("tickets", "read"), app.apiTicketAction)
	authorized.GET("/tickets/:id/comments", app.requirePermission("tickets", "read"), app.apiTicketComments)
	authorized.POST("/tickets/:id/comments", app.requirePermission("tickets", "read"), app.apiAddTicketComment)
	authorized.GET("/tickets/:id/events", app.requirePermission("tickets", "read"), app.apiTicketEvents)
	authorized.GET("/approvals", app.requirePermission("tickets", "approve"), app.apiApprovals)
	authorized.GET("/ticket-types", app.requirePermission("tickets", "read"), app.apiTicketTypes)
	authorized.POST("/ticket-types", app.requirePermission("ticket_types", "create"), app.apiCreateTicketType)
	authorized.PATCH("/ticket-types/:id", app.requirePermission("ticket_types", "update"), app.apiUpdateTicketType)
	authorized.DELETE("/ticket-types/:id", app.requirePermission("ticket_types", "delete"), app.apiDeleteTicketType)

	// RBAC administration.
	authorized.GET("/rbac/catalog", app.apiRbacCatalog)
	authorized.GET("/roles", app.requirePermission("roles", "read"), app.apiRoles)
	authorized.POST("/roles", app.requirePermission("roles", "create"), app.apiCreateRole)
	authorized.PUT("/roles/:key", app.requirePermission("roles", "update"), app.apiUpdateRole)
	authorized.DELETE("/roles/:key", app.requirePermission("roles", "delete"), app.apiDeleteRole)
	authorized.GET("/roles/:key/permissions", app.requirePermission("roles", "read"), app.apiRolePermissions)
	authorized.PUT("/roles/:key/permissions", app.requirePermission("roles", "update"), app.apiSetRolePermissions)
	authorized.GET("/policies", app.requirePermission("policies", "read"), app.apiPolicies)
	authorized.POST("/policies", app.requirePermission("policies", "create"), app.apiAddPolicy)
	authorized.DELETE("/policies", app.requirePermission("policies", "delete"), app.apiRemovePolicy)
	authorized.GET("/users/:id/roles", app.requirePermission("users", "read"), app.apiUserRoles)
	authorized.POST("/users/:id/roles", app.requirePermission("users", "update"), app.apiAssignUserRole)
	authorized.DELETE("/users/:id/roles", app.requirePermission("users", "update"), app.apiUnassignUserRole)
	authorized.GET("/orgs", app.requirePermission("orgs", "read"), app.apiOrgs)
	authorized.POST("/orgs", app.requirePermission("orgs", "create"), app.apiCreateOrg)
	authorized.GET("/orgs/:id", app.requirePermission("orgs", "read"), app.apiGetOrg)
	authorized.PUT("/orgs/:id", app.requirePermission("orgs", "update"), app.apiUpdateOrg)
	authorized.DELETE("/orgs/:id", app.requirePermission("orgs", "delete"), app.apiDeleteOrg)
	authorized.GET("/orgs/:id/members", app.requirePermission("orgs", "read"), app.apiOrgMembers)
	authorized.POST("/orgs/:id/members", app.requirePermission("orgs", "update"), app.apiAddOrgMember)
	authorized.DELETE("/orgs/:id/members/:userId", app.requirePermission("orgs", "update"), app.apiRemoveOrgMember)

	// OAuth link flows stay on the server: redirects and callbacks.
	r.GET("/auth/:provider/login", app.apiAuthHandler(app.oauthLogin))
	r.GET("/auth/:provider/callback", app.oauthCallback)
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

// notRoot serves problem+json for API paths and falls back to the SPA for
// everything else (client-side routes have no file on disk).
func (a *App) notFound(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.Header("X-Content-Type-Options", "nosniff")
		a.problem(c, http.StatusNotFound, "not found", nil)
		return
	}
	serveSPA(c)
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

// apiAuthRequired resolves the caller for JSON endpoints; failures are 401,
// never a redirect: browsers talking to the SPA read the status, not
// Location. Bearer values with the API-key prefix resolve through the key
// store instead of the session store and carry their scopes in the context.
// Used as group middleware on the authorized API subgroup.
func (a *App) apiAuthRequired(c *gin.Context) {
	token := a.sessionToken(c)
	if strings.HasPrefix(token, service.APIKeyPrefix) {
		key, _, err := a.services.APIKeys.Resolve(c.Request.Context(), token)
		if err != nil {
			a.problem(c, http.StatusUnauthorized, "unauthorized", nil)
			c.Abort()
			return
		}
		// Keys live outside the session store; resolve the owner directly.
		user, userErr := a.services.Users.Get(c.Request.Context(), key.UserID)
		if userErr != nil {
			a.problem(c, http.StatusUnauthorized, "unauthorized", nil)
			c.Abort()
			return
		}
		c.Set("user_id", user.ID)
		c.Set("user", user)
		c.Set("auth_scopes", key.Scopes)
		return
	}
	user, err := a.services.Auth.CurrentUser(c.Request.Context(), token)
	if err != nil {
		a.problem(c, http.StatusUnauthorized, "unauthorized", nil)
		c.Abort()
		return
	}
	c.Set("user_id", user.ID)
	c.Set("user", user)
}

// apiAuthHandler guards the server-side OAuth redirect routes, which still
// answer redirects rather than problem+json.
func (a *App) apiAuthHandler(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := a.services.Auth.CurrentUser(c.Request.Context(), a.sessionToken(c))
		if err != nil {
			c.Redirect(http.StatusSeeOther, "/login")
			return
		}
		c.Set("user_id", user.ID)
		c.Set("user", user)
		next(c)
	}
}

// currentUser returns the user resolved by apiAuth.
func (a *App) currentUser(c *gin.Context) model.User {
	user, _ := c.Get("user")
	return user.(model.User)
}

// setSession issues the session cookie with the hardening flags; the TTL
// itself is enforced server-side.
func (a *App) setSession(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("app_session", token, int(a.cfg.SessionTTL.Seconds()), "/", "", a.cfg.CookieSecure, true)
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

func listOptions(c *gin.Context) repository.ListOptions {
	return repository.ListOptions{
		Query: c.Query("q"), Mode: c.DefaultQuery("mode", "offset"), Limit: intQuery(c, "limit", 25),
		Offset: intQuery(c, "offset", 0), After: c.Query("after"),
		Status: c.Query("status"), Type: c.Query("type"),
	}
}
