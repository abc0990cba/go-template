package service

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/casbin/casbin/v2"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/platform/mailer"
)

// Deps bundles the ports the services run on. One store usually satisfies
// every repo, so callers pass the same adapter for each; a memory fake and
// a PostgreSQL adapter are interchangeable. Queue and RBACEnforcer may be
// nil in the single-process development mode (jobs run inline; RBAC then
// denies everything, which keeps non-HTTP processes safe).
type Deps struct {
	Auth         AuthRepo
	Users        UsersRepo
	Jobs         JobsRepo
	Reports      ReportsRepo
	Analytics    AnalyticsRepo
	Orgs         OrgsRepo
	Roles        RolesRepo
	Audit        AuditRepo
	Settings     SettingsRepo
	Notifications NotificationsRepo
	EmailTokens  EmailTokensRepo
	APIKeys      APIKeysRepo
	Webhooks     WebhooksRepo
	WebhookDispatcher WebhookDispatcher
	Mailer       mailer.Sender
	// Realtime publishes live notifications to connected browsers; nil in
	// processes without an SSE hub.
	Realtime     func(model.Notification)
	Events       EventSink
	EventStats   EventStats
	Queue        JobQueue
	Observer     JobObserver
	RBACEnforcer *casbin.SyncedEnforcer
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
			client:         &http.Client{Timeout: cfg.ExternalTimeout},
			maxConcurrency: cfg.MaxConcurrency,
		},
		Reports:   &ReportService{store: deps.Reports},
		Analytics: &AnalyticsService{store: deps.Analytics, events: deps.EventStats, logger: logger},
		RBAC:      &RBACService{enforcer: deps.RBACEnforcer, roles: deps.Roles, orgs: deps.Orgs, logger: logger},
		Audit:     &AuditService{store: deps.Audit, logger: logger},
		Settings:  &SettingsService{store: deps.Settings},
	}
	notifications := &NotificationsService{store: deps.Notifications, publish: deps.Realtime}
	services.Notifications = notifications
	services.Jobs.WithNotifications(notifications)
	services.Email = NewEmailService(deps.EmailTokens, deps.Auth, deps.Mailer, cfg.BaseURL, logger)
	services.APIKeys = NewAPIKeysService(deps.APIKeys)
	services.Webhooks = NewWebhooksService(deps.Webhooks, deps.WebhookDispatcher, services.Settings, &http.Client{Timeout: cfg.ExternalTimeout}, logger)
	services.Email.WithNotifier(func(ctx context.Context, userID, typ, title, body, link string) {
		_ = notifications.Create(ctx, userID, typ, title, body, link)
	})
	return services
}
