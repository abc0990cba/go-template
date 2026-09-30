package service

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/casbin/casbin/v2"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/platform/mailer"
	"github.com/your-org/go-template/internal/reliability"
)

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
	Auth          AuthRepo
	Users         UsersRepo
	Jobs          JobsRepo
	Reports       ReportsRepo
	Analytics     AnalyticsRepo
	Orgs          OrgsRepo
	Roles         RolesRepo
	Audit         AuditRepo
	Settings      SettingsRepo
	Notifications NotificationsRepo
	EmailTokens   EmailTokensRepo
	APIKeys       APIKeysRepo
	Webhooks      WebhooksRepo
	Tickets       TicketsRepo
	Streams       StreamsRepo
	MFA           MFARepo
	Tokens        TokenRepo
	Search        SearchRepo
	Invitations   InvitationsRepo
	// Files stores file metadata; Blobs is the backing file store. Both may
	// be nil in processes that do not serve uploads.
	Files FilesRepo
	Blobs FileStore
	// Cache is the optional read-through cache (memory or Redis); nil keeps
	// every read on the store.
	Cache             Cache
	WebhookDispatcher WebhookDispatcher
	// Breakers wraps outbound HTTP and broker calls; nil disables breakers.
	Breakers *reliability.Breakers
	// Bulkheads bound concurrent calls per outbound dependency; nil runs
	// unbounded (the same nil-safe convention as Breakers).
	Bulkheads *reliability.Bulkheads
	Mailer    mailer.Sender
	// Realtime publishes live notifications to connected browsers; nil in
	// processes without an SSE hub.
	Realtime        func(model.Notification)
	Events          EventSink
	EventStats      EventStats
	Queue           JobQueue
	Observer        JobObserver
	StreamsObserver StreamsObserver
	RBACEnforcer    *casbin.SyncedEnforcer
}

// Container wires every service against the store ports; handlers depend on
// this aggregate rather than on individual services.
type Container struct {
	Auth          *AuthService
	Users         *UserService
	Jobs          *JobService
	Reports       *ReportService
	Analytics     *AnalyticsService
	RBAC          *RBACService
	Audit         *AuditService
	Settings      *SettingsService
	Notifications *NotificationsService
	Email         *EmailService
	APIKeys       *APIKeysService
	Webhooks      *WebhooksService
	Tickets       *TicketsService
	Streams       *StreamsService
	MFA           *MFAService
	Tokens        *TokenService
	Search        *SearchService
	Invitations   *InvitationsService
	Files         *FilesService
	Exports       *ExportService
}

// NewContainer builds the service graph from the dependency set.
func NewContainer(deps Deps, logger *slog.Logger, cfg config.Config) *Container {
	services := &Container{
		Auth:  &AuthService{store: deps.Auth, events: deps.Events, sessionTTL: cfg.SessionTTL},
		Users: &UserService{store: deps.Users},
		Jobs: &JobService{
			store:          deps.Jobs,
			reports:        deps.Reports,
			queue:          deps.Queue,
			observer:       deps.Observer,
			events:         deps.Events,
			logger:         logger,
			client:         tracedClient(cfg.ExternalTimeout),
			maxConcurrency: cfg.MaxConcurrency,
		},
		Reports:   &ReportService{store: deps.Reports},
		Analytics: &AnalyticsService{store: deps.Analytics, events: deps.EventStats, logger: logger},
		RBAC:      (&RBACService{enforcer: deps.RBACEnforcer, roles: deps.Roles, orgs: deps.Orgs, logger: logger}).WithCache(deps.Cache),
		Audit:     &AuditService{store: deps.Audit, logger: logger},
		Settings:  &SettingsService{store: deps.Settings},
	}
	services.Streams = NewStreamsService(deps.Streams, deps.StreamsObserver, logger)
	services.Search = NewSearchService(deps.Search, services.RBAC)
	services.Invitations = NewInvitationsService(deps.Invitations, deps.Orgs, deps.Auth, services.RBAC, deps.Mailer, cfg.BaseURL, logger)
	services.Jobs.breakers = deps.Breakers
	services.Jobs.bulkheads = deps.Bulkheads
	notifications := &NotificationsService{store: deps.Notifications, publish: deps.Realtime}
	services.Notifications = notifications
	services.Jobs.WithNotifications(notifications)
	if deps.Cache != nil {
		services.Users.WithCache(deps.Cache).WithLogger(logger)
		services.Auth.WithCache(deps.Cache)
	}
	services.MFA = NewMFAService(deps.MFA, deps.Auth, services.Settings, deps.Events, cfg.SessionTTL)
	services.Tokens = NewTokenService(deps.Tokens, deps.Auth, services.MFA, deps.Events, cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL)
	services.Tickets = &TicketsService{store: deps.Tickets, rbac: services.RBAC, events: deps.Events, logger: logger}
	if deps.Files != nil && deps.Blobs != nil {
		services.Files = NewFilesService(deps.Files, deps.Blobs, services.Tickets, cfg.FilesMaxBytes, logger)
		services.Exports = NewExportService(deps.Users, deps.Auth, deps.Tickets, deps.Notifications, deps.Audit, deps.Files, deps.Blobs)
	}
	services.Tickets.WithNotifications(notifications)
	services.Email = NewEmailService(deps.EmailTokens, deps.Auth, deps.Mailer, cfg.BaseURL, logger)
	services.APIKeys = NewAPIKeysService(deps.APIKeys)
	services.Webhooks = NewWebhooksService(deps.Webhooks, deps.WebhookDispatcher, services.Settings, tracedClient(cfg.ExternalTimeout), deps.Breakers, deps.Bulkheads, logger)
	services.Email.WithNotifier(func(ctx context.Context, userID, typ, title, body, link string) {
		_ = notifications.Create(ctx, userID, typ, title, body, link)
	})
	return services
}
