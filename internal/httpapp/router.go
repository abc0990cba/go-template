// Package httpapp contains the HTTP layer: routing, middleware, the JSON
// API handlers, the SSE stream, and the embedded SPA.
package httpapp

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/apikeys"
	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/bootstrap"
	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/export"
	"github.com/your-org/go-template/internal/files"
	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/invitations"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/mfa"
	"github.com/your-org/go-template/internal/notifications"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/platform/oauth"
	"github.com/your-org/go-template/internal/platform/ratelimit"
	"github.com/your-org/go-template/internal/rbac"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/search"
	"github.com/your-org/go-template/internal/settings"
	"github.com/your-org/go-template/internal/streams"
	"github.com/your-org/go-template/internal/tickets"
	"github.com/your-org/go-template/internal/tokens"
	"github.com/your-org/go-template/internal/users"
	"github.com/your-org/go-template/internal/webhooks"
)

// App carries the handler dependencies; all handlers are methods on it.
type App struct {
	cfg      config.Config
	services *bootstrap.Container
	logger   *slog.Logger
	pinger   kernel.Pinger
	oauth    *oauth.Manager
	hub      *realtime.Hub
	// limiters shed excess traffic per scope; memory in dev, Redis-backed
	// when the factory is wired.
	limiters map[string]ratelimit.Limiter
	// idempotencyStore stores Idempotency-Key reservations; nil disables it.
	idempotencyStore kernel.IdempotencyRepo
	// readyProbes are extra readiness checks (broker pings), run after the
	// store ping.
	readyProbes []func(ctx context.Context) error
}

// Options carries the HTTP layer's optional dependencies; zero values keep
// the previous behavior (in-memory rate limiting, no idempotency, store-only
// readiness, the config-built OAuth manager).
type Options struct {
	// LimiterFactory builds one limiter per scope ("api", "auth"); nil uses
	// in-memory token buckets.
	LimiterFactory func(scope string, rps float64, burst int) ratelimit.Limiter
	Idempotency    kernel.IdempotencyRepo
	DemoEnricher   streams.DemoPublisher
	ReadyProbes    []func(ctx context.Context) error
	// OAuth replaces the manager built from config (tests point it at mock
	// provider endpoints without rebuilding the router).
	OAuth *oauth.Manager
}

// NewRouter builds the complete Gin engine: middleware, health endpoints,
// OpenAPI docs, the /api group with the SSE stream, and the embedded SPA.
func NewRouter(cfg config.Config, services *bootstrap.Container, logger *slog.Logger, pinger kernel.Pinger, build observ.Build, hub *realtime.Hub, opts ...Options) http.Handler {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	limiters := map[string]ratelimit.Limiter{
		"api":  ratelimit.NewMemory(cfg.APIRateRPS, cfg.APIRateBurst),
		"auth": ratelimit.NewMemory(cfg.AuthRateRPS, cfg.AuthRateBurst),
		"user": ratelimit.NewMemory(cfg.UserRateRPS, cfg.UserRateBurst),
	}
	if o.LimiterFactory != nil {
		limiters["api"] = o.LimiterFactory("api", cfg.APIRateRPS, cfg.APIRateBurst)
		limiters["auth"] = o.LimiterFactory("auth", cfg.AuthRateRPS, cfg.AuthRateBurst)
		limiters["user"] = o.LimiterFactory("user", cfg.UserRateRPS, cfg.UserRateBurst)
	}
	oauthManager := o.OAuth
	if oauthManager == nil {
		oauthManager = oauth.NewManager(cfg)
	}
	app := &App{
		cfg: cfg, services: services, logger: logger, pinger: pinger,
		oauth: oauthManager, hub: hub,
		limiters:         limiters,
		idempotencyStore: o.Idempotency,
		readyProbes:      o.ReadyProbes,
	}
	r := gin.New()
	r.Use(gin.Recovery(), app.securityHeaders(), app.requestID(), app.tracing(), app.metrics(), app.accessLog(), app.maintenanceGate(), app.bodyLimit())
	r.NoRoute(app.notFound)
	// The prerendered marketing landing owns "/"; the SPA keeps every other
	// extension-less path through the NoRoute fallback (see notFound).
	r.GET("/", app.landing)
	r.HEAD("/", app.landing)
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
	// Every /api request passes the shared rate limiter; auth endpoints add
	// a much tighter per-IP budget on top.
	api := r.Group("/api", app.originCheck(), app.apiRateLimit())

	// Authenticated routes resolve the caller, then the active Casbin
	// domain (X-Org-Id header, else the user's single org, else the
	// platform). Guarded routes additionally require a catalog permission;
	// self-service routes (me, profile, sessions, SSE) only need auth. The
	// identity limiter throttles each principal on top of the per-IP budget
	// the whole /api group already paid.
	authorized := api.Group("", app.apiAuthRequired, app.identityRateLimit(), app.orgDomain)
	authorized.GET("/events/stream", app.sseStream)

	// Feature surfaces registered from their own packages.
	auth.RegisterHTTP(api, authorized, auth.HTTP{
		Svc: services.Auth, RBAC: services.RBAC, Users: services.Users, MFA: services.MFA,
		Notifications: services.Notifications, Audit: services.Audit, Settings: services.Settings,
		OAuth: app.oauth, Logger: logger,
		AuthRateLimit: app.authRateLimit(), RequirePermission: app.requirePermission,
		SessionTTL: cfg.SessionTTL, CookieSecure: cfg.CookieSecure, OAuthStateTTL: cfg.OAuthStateTTL,
	})
	rbac.RegisterHTTP(authorized, rbac.HTTP{
		Svc: services.RBAC, Audit: services.Audit, Logger: logger,
		RequirePermission: app.requirePermission,
	})
	invitations.RegisterHTTP(authorized, invitations.HTTP{
		Svc: services.Invitations, Audit: services.Audit, Logger: logger,
		RequirePermission: app.requirePermission,
	})
	email.RegisterHTTP(api, email.HTTP{Svc: services.Email, Settings: services.Settings, Audit: services.Audit, AuthRateLimit: app.authRateLimit()})
	mfa.RegisterHTTP(api, authorized, mfa.HTTP{
		Svc: services.MFA, Settings: services.Settings, Notifications: services.Notifications,
		Audit: services.Audit, Logger: logger,
		AuthRateLimit: app.authRateLimit(), RequirePermission: app.requirePermission,
		SessionTTL: cfg.SessionTTL, CookieSecure: cfg.CookieSecure,
	})
	apikeys.RegisterHTTP(authorized, apikeys.HTTP{Svc: services.APIKeys, Audit: services.Audit})
	audit.RegisterHTTP(authorized, audit.HTTP{Svc: services.Audit, RequirePermission: app.requirePermission})
	notifications.RegisterHTTP(authorized, notifications.HTTP{Svc: services.Notifications})
	settings.RegisterHTTP(api, authorized, settings.HTTP{Svc: services.Settings, Audit: services.Audit, RequirePermission: app.requirePermission})
	analytics.RegisterHTTP(authorized, analytics.HTTP{
		Reports: services.Reports, Analytics: services.Analytics,
		// Jobs is a func, not a typed port: the jobs package imports
		// analytics for report persistence, so a signature naming jobs.Job
		// here would cycle the import graph.
		Jobs: func(ctx context.Context, typ, payload, createdBy string) (any, error) {
			return services.Jobs.Enqueue(ctx, typ, payload, createdBy)
		},
		RequirePermission: app.requirePermission, ETag: app.etag,
	})
	jobs.RegisterHTTP(authorized, jobs.HTTP{
		Svc: services.Jobs, Logger: logger,
		RequirePermission: app.requirePermission, ETag: app.etag,
	})
	files.RegisterHTTP(authorized, files.HTTP{
		Svc: services.Files, Tickets: services.Tickets, Audit: services.Audit, Logger: logger,
		RequirePermission: app.requirePermission,
	})
	tickets.RegisterHTTP(authorized, tickets.HTTP{
		Svc: services.Tickets, RBAC: services.RBAC, Audit: services.Audit, Logger: logger,
		RequirePermission: app.requirePermission,
		ETag:              app.etag, Idempotency: app.idempotency,
	})
	webhooks.RegisterHTTP(authorized, webhooks.HTTP{
		Svc: services.Webhooks, Audit: services.Audit, Logger: logger,
		RequirePermission: app.requirePermission,
	})
	export.RegisterHTTP(authorized, export.HTTP{
		Svc: services.Exports, Jobs: services.Jobs, Audit: services.Audit, Logger: logger,
	})
	users.RegisterHTTP(authorized, users.HTTP{
		Svc: services.Users, Logger: logger,
		// Audit is a closure over audit.Record, not a *audit.Service field:
		// audit reads the users.User actor out of the request context, so a
		// users→audit import would cycle the graph.
		Audit: func(c *gin.Context, action, resourceType, resourceID string, before, after any) {
			audit.Record(services.Audit, c, action, resourceType, resourceID, before, after)
		},
		RequirePermission: app.requirePermission, ETag: app.etag,
	})
	streams.RegisterHTTP(authorized, streams.HTTP{
		Svc: services.Streams, Users: services.Users, Demo: o.DemoEnricher,
		Audit: services.Audit, Logger: logger,
		RequirePermission: app.requirePermission,
	})
	search.RegisterHTTP(authorized, search.HTTP{
		Svc: services.Search, Logger: logger,
	})

	// OAuth-style token grants for non-browser clients; opt-in per
	// deployment via APP_JWT_SECRET (otherwise 501).
	tokens.RegisterHTTP(api, tokens.HTTP{
		Svc: services.Tokens, Audit: services.Audit, Logger: logger,
		AuthRateLimit: app.authRateLimit(),
	})

	// OAuth link flows stay on the server: redirects and callbacks (outside
	// /api, per the SPA's contract).
	auth.RegisterRoot(r, auth.HTTP{
		Svc: services.Auth, OAuth: app.oauth, Logger: logger,
		SessionTTL: cfg.SessionTTL, CookieSecure: cfg.CookieSecure, OAuthStateTTL: cfg.OAuthStateTTL,
	})
	return r
}

func (a *App) ok(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ready reflects dependency health; a failing check yields 503. The store
// ping runs first, then the optional probes (broker pings) — the API is only
// "ready" when the infrastructure the operators configured is reachable.
func (a *App) ready(c *gin.Context) {
	if err := a.pinger.Ping(c.Request.Context()); err != nil {
		a.logger.Error("readiness check failed", "err", err)
		a.problem(c, http.StatusServiceUnavailable, "not ready", nil)
		return
	}
	for _, probe := range a.readyProbes {
		if err := probe(c.Request.Context()); err != nil {
			a.logger.Error("readiness probe failed", "err", err)
			a.problem(c, http.StatusServiceUnavailable, "not ready", nil)
			return
		}
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
	httpx.Problem(c, status, title, err)
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
// store instead of the session store and carry their scopes in the context;
// JWT-shaped bearers (two dots) verify through the token grant when that
// feature is configured. Used as group middleware on the authorized API
// subgroup.
func (a *App) apiAuthRequired(c *gin.Context) {
	token := a.sessionToken(c)
	if strings.HasPrefix(token, apikeys.Prefix) {
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
	if strings.Count(token, ".") == 2 {
		a.jwtAuth(c, token)
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

// jwtAuth resolves an access JWT to its user. The token carries identity
// only; permissions are still resolved server-side per request.
func (a *App) jwtAuth(c *gin.Context, token string) {
	userID, err := a.services.Tokens.VerifyAccess(token)
	if err != nil {
		a.problem(c, http.StatusUnauthorized, "unauthorized", nil)
		c.Abort()
		return
	}
	user, err := a.services.Users.Get(c.Request.Context(), userID)
	if err != nil {
		a.problem(c, http.StatusUnauthorized, "unauthorized", nil)
		c.Abort()
		return
	}
	c.Set("user_id", user.ID)
	c.Set("user", user)
}

// currentUser returns the user resolved by apiAuth.
func (a *App) currentUser(c *gin.Context) users.User {
	user, _ := c.Get("user")
	return user.(users.User)
}
