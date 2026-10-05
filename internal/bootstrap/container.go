package bootstrap

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/casbin/casbin/v2"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/apikeys"
	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/cache"
	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/export"
	"github.com/your-org/go-template/internal/files"
	"github.com/your-org/go-template/internal/invitations"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/mfa"
	"github.com/your-org/go-template/internal/notifications"
	"github.com/your-org/go-template/internal/platform/mailer"
	"github.com/your-org/go-template/internal/rbac"
	"github.com/your-org/go-template/internal/reliability"
	"github.com/your-org/go-template/internal/search"
	"github.com/your-org/go-template/internal/settings"
	"github.com/your-org/go-template/internal/streams"
	"github.com/your-org/go-template/internal/tickets"
	"github.com/your-org/go-template/internal/tokens"
	"github.com/your-org/go-template/internal/users"
	"github.com/your-org/go-template/internal/webhooks"
)

// The settings service satisfies the webhooks and mfa feature flag
// dependencies.
var (
	_ webhooks.Settings = (*settings.Service)(nil)
	_ mfa.Settings      = (*settings.Service)(nil)
)

// The export feature reads the not-yet-migrated stores through narrow
// consumer-side interfaces; the Deps ports satisfy them structurally.
var (
	_ export.Users         = users.Store(nil)
	_ export.Auth          = auth.Store(nil)
	_ export.Tickets       = tickets.Store(nil)
	_ export.Notifications = notifications.Store(nil)
	_ export.AuditTrail    = audit.Store(nil)
	_ export.FileMeta      = files.Store(nil)
)

// The token grant resolves credentials through a narrow view of the auth
// store; the Deps port satisfies it structurally.
var _ tokens.Auth = auth.Store(nil)

// tracedClient builds the outbound HTTP client used for enrichment and
// webhook deliveries: bounded by the external timeout and instrumented with
// OpenTelemetry client spans (the global tracer provider), so third-party
// calls appear in the trace that caused them.
func tracedClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
}

// Deps bundles the ports the services run on. One store usually satisfies
// every repo, so callers pass the same adapter for each; a memory fake and
// a PostgreSQL adapter are interchangeable. Queue and RBACEnforcer may be
// nil in the single-process development mode (jobs run inline; RBAC then
// denies everything, which keeps non-HTTP processes safe).
type Deps struct {
	Auth          auth.Store
	Users         users.Store
	Jobs          jobs.Store
	Reports       analytics.ReportStore
	Orgs          rbac.OrgsStore
	Roles         rbac.RolesStore
	Audit         audit.Store
	Settings      settings.Store
	Notifications notifications.Store
	EmailTokens   email.Store
	APIKeys       apikeys.Store
	Webhooks      webhooks.Store
	Tickets       tickets.Store
	Streams       streams.Store
	MFA           mfa.Store
	Tokens        tokens.Store
	Search        search.Store
	Invitations   invitations.Store
	// Files stores file metadata; Blobs is the backing file store. Both may
	// be nil in processes that do not serve uploads.
	Files files.Store
	Blobs files.FileStore
	// Cache is the optional read-through cache (memory or Redis); nil keeps
	// every read on the store.
	Cache             cache.Cache
	WebhookDispatcher webhooks.Dispatcher
	// EmailMessages backs the delivery log; nil keeps email on the
	// direct-send path with no log. EmailDispatcher (optional) hands
	// recorded messages to the queue instead of sending inline.
	EmailMessages   email.MessageStore
	EmailDispatcher email.Dispatcher
	// LoginGuard backs progressive lockout; nil (or threshold 0) keeps
	// login free of account-state tracking.
	LoginGuard auth.LoginGuard
	// Pwned is the optional breached-password gate for new passwords; nil
	// keeps registrations and resets free of external checks.
	Pwned kernel.PwnedChecker
	// JWTKeys enables kid-based token-secret rotation; nil keeps
	// single-secret mode (APP_JWT_SECRET).
	JWTKeys *tokens.KeySet
	// JWTRevocations is the shared jti denylist (Redis in production,
	// in-process otherwise); nil keeps access verification stateless.
	JWTRevocations tokens.Revocations
	// Breakers wraps outbound HTTP and broker calls; nil disables breakers.
	Breakers *reliability.Breakers
	// Bulkheads bound concurrent calls per outbound dependency; nil runs
	// unbounded (the same nil-safe convention as Breakers).
	Bulkheads *reliability.Bulkheads
	Mailer    mailer.Sender
	// Realtime publishes live notifications to connected browsers; nil in
	// processes without an SSE hub.
	Realtime        func(notifications.Notification)
	Events          analytics.Sink
	EventStats      analytics.Stats
	Queue           jobs.Queue
	Observer        jobs.Observer
	StreamsObserver streams.Observer
	RBACEnforcer    *casbin.SyncedEnforcer
}

// Container wires every service against the store ports; handlers depend on
// this aggregate rather than on individual services.
type Container struct {
	Auth          *auth.Service
	Users         *users.Service
	Jobs          *jobs.Service
	Reports       *analytics.ReportService
	Analytics     *analytics.Service
	RBAC          *rbac.Service
	Audit         *audit.Service
	Settings      *settings.Service
	Notifications *notifications.Service
	Email         *email.Service
	APIKeys       *apikeys.Service
	Webhooks      *webhooks.Service
	Tickets       *tickets.Service
	Streams       *streams.Service
	MFA           *mfa.Service
	Tokens        *tokens.Service
	Search        *search.Service
	Invitations   *invitations.Service
	Files         *files.Service
	Exports       *export.Service
}

// NewContainer builds the service graph from the dependency set.
func NewContainer(deps Deps, logger *slog.Logger, cfg config.Config) *Container {
	services := &Container{
		Auth:  auth.NewService(deps.Auth, deps.Events, cfg.SessionTTL),
		Users: users.NewService(deps.Users),
		Jobs: jobs.NewService(deps.Jobs, deps.Reports, deps.Queue, deps.Observer, deps.Events,
			tracedClient(cfg.ExternalTimeout), deps.Breakers, deps.Bulkheads, cfg.MaxConcurrency, logger),
		Reports:   analytics.NewReportService(deps.Reports),
		Analytics: analytics.NewService(deps.Reports, deps.EventStats, logger),
		RBAC:      rbac.NewService(deps.RBACEnforcer, deps.Roles, deps.Orgs, logger).WithCache(deps.Cache),
		Audit:     audit.NewService(deps.Audit),
		Settings:  settings.NewService(deps.Settings),
	}
	services.Streams = streams.NewService(deps.Streams, deps.StreamsObserver, logger)
	services.Search = search.NewService(deps.Search, services.RBAC)
	services.Invitations = invitations.NewService(deps.Invitations, deps.Orgs, deps.Auth, services.RBAC, deps.Mailer, cfg.BaseURL, logger)
	notifications := notifications.NewService(deps.Notifications, deps.Realtime)
	services.Notifications = notifications
	services.Jobs.WithNotifications(notifications)
	if deps.Cache != nil {
		services.Users.WithCache(deps.Cache).WithLogger(logger)
		services.Auth.WithCache(deps.Cache)
	}
	services.MFA = mfa.NewService(deps.MFA, deps.Auth, services.Settings, deps.Events, cfg.SessionTTL, auth.VerifyPassword)
	services.Tokens = tokens.NewService(deps.Tokens, deps.Auth, services.MFA, deps.Events, cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL, auth.VerifyPassword)
	services.Tokens.WithKeys(deps.JWTKeys)
	services.Tokens.WithRevocations(deps.JWTRevocations)
	services.Tickets = tickets.NewService(deps.Tickets, services.RBAC, deps.Events, logger)
	if deps.Files != nil && deps.Blobs != nil {
		services.Files = files.NewService(deps.Files, deps.Blobs, services.Tickets, cfg.FilesMaxBytes, logger)
		services.Exports = export.NewService(deps.Users, deps.Auth, deps.Tickets, deps.Notifications, deps.Audit, deps.Files, deps.Blobs)
	}
	services.Tickets.WithNotifications(notifications)
	services.Email = email.NewService(deps.EmailTokens, deps.Auth, deps.Mailer, auth.HashPassword, cfg.BaseURL, logger)
	services.Email.WithMessages(deps.EmailMessages, deps.EmailDispatcher)
	services.Auth.WithLockout(deps.LoginGuard, cfg.LockoutThreshold, cfg.LockoutWindow)
	services.Auth.WithPwned(deps.Pwned)
	services.Email.WithPwned(deps.Pwned)
	services.APIKeys = apikeys.NewService(deps.APIKeys)
	services.Webhooks = webhooks.NewService(deps.Webhooks, deps.WebhookDispatcher, services.Settings, tracedClient(cfg.ExternalTimeout), deps.Breakers, deps.Bulkheads, logger)
	services.Email.WithNotifier(func(ctx context.Context, userID, typ, title, body, link string) {
		_ = notifications.Create(ctx, userID, typ, title, body, link)
	})
	return services
}
