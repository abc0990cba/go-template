// Package bootstrap selects and opens the store adapter from configuration:
// PostgreSQL when APP_DATABASE_URL is set, the in-memory fake otherwise.
package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/model"
	ch "github.com/your-org/go-template/internal/platform/clickhouse"
	"github.com/your-org/go-template/internal/platform/postgres"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

// Repos bundles the store ports; a single adapter fills them all.
type Repos struct {
	Auth          service.AuthRepo
	Users         service.UsersRepo
	Jobs          service.JobsRepo
	Reports       service.ReportsRepo
	Analytics     service.AnalyticsRepo
	Orgs          service.OrgsRepo
	Roles         service.RolesRepo
	Audit         service.AuditRepo
	Settings      service.SettingsRepo
	Notifications service.NotificationsRepo
	EmailTokens   service.EmailTokensRepo
	APIKeys       service.APIKeysRepo
	Webhooks      service.WebhooksRepo
	Tickets       service.TicketsRepo
	Streams       service.StreamsRepo
	Idempotency   service.IdempotencyRepo
	MFA           service.MFARepo
	Tokens        service.TokenRepo
	Search        service.SearchRepo
	Rules         repository.RuleStore
	Pinger        service.Pinger
	// Events receives the analytics event trail; nil means the process does
	// not persist events (plain PostgreSQL mode).
	Events service.EventSink
	// EventStats reads aggregates from ClickHouse; nil without ClickHouse.
	EventStats service.EventStats
	// Warehouse accepts historical event batches (the demo seed backfill);
	// nil when events cannot be written in bulk.
	Warehouse EventBackfiller
	Kind      string
}

// EventBackfiller writes a batch of historical events in one insert.
type EventBackfiller interface {
	InsertEvents(ctx context.Context, events []model.EventRecord) error
	// DeleteDemoEvents removes seed-generated events (demo reset).
	DeleteDemoEvents(ctx context.Context) error
}

// Store opens the configured adapters. The returned function releases them.
// ClickHouse is opened on top of the operational store when
// APP_CLICKHOUSE_ADDR is set; its failure to open is logged, not fatal —
// the app runs without the analytics warehouse.
func Store(cfg config.Config, logger *slog.Logger) (Repos, func(), error) {
	var (
		events   service.EventSink
		stats    service.EventStats
		closeAll = func() {}
	)
	var client *ch.Client
	if cfg.ClickHouseAddr != "" {
		opened, err := ch.Open(context.Background(), cfg.ClickHouseAddr, cfg.ClickHouseDB, cfg.ClickHouseUser, cfg.ClickHousePass, os.DirFS("migrations/clickhouse"))
		if err != nil {
			logger.Warn("clickhouse unavailable; events are not persisted", "err", err)
		} else {
			client = opened
			buffer := client.NewEventBuffer(logger, 500, 2*time.Second)
			events = buffer
			stats = client
			closeAll = func() { _ = buffer.Close(); _ = client.Close() }
		}
	}

	if cfg.DatabaseURL == "" {
		store := repository.NewMemoryStore()
		if events != nil {
			events = compoundSink{store, events}
		} else {
			events = store
		}
		return Repos{
			Auth: store, Users: store, Jobs: store, Reports: store, Analytics: store,
			Orgs: store, Roles: store, Audit: store, Settings: store, Notifications: store, EmailTokens: store, APIKeys: store, Webhooks: store, Tickets: store, Streams: store, Idempotency: store, Rules: store, MFA: store, Tokens: store, Search: store,
			Pinger: store, Events: events, EventStats: store, Warehouse: store, Kind: "memory",
		}, closeAll, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var opts []postgres.Option
	if cfg.OTLPEndpoint != "" {
		opts = append(opts, postgres.WithQueryTracing())
	}
	opts = append(opts, postgres.WithTuning(postgres.Tuning{
		MaxConns:          cfg.Pool.MaxConns,
		MinConns:          cfg.Pool.MinConns,
		ConnMaxLifetime:   cfg.Pool.ConnMaxLifetime,
		ConnMaxIdleTime:   cfg.Pool.ConnMaxIdleTime,
		HealthCheckPeriod: cfg.Pool.HealthCheckPeriod,
		StatementTimeout:  cfg.Pool.StatementTimeout,
		IdleTxTimeout:     cfg.Pool.IdleTxTimeout,
	}))
	store, err := postgres.Open(ctx, cfg.DatabaseURL, os.DirFS("migrations/postgres"), opts...)
	if err != nil {
		return Repos{}, func() {}, err
	}
	logger.Info("postgres store ready", "migrations", "applied")
	var warehouse EventBackfiller
	if client != nil {
		warehouse = client
	}
	return Repos{
		Auth: store, Users: store, Jobs: store, Reports: store, Analytics: store,
		Orgs: store, Roles: store, Audit: store, Settings: store, Notifications: store, EmailTokens: store, APIKeys: store, Webhooks: store, Tickets: store, Streams: store, Idempotency: store, Rules: store, MFA: store, Tokens: store, Search: store,
		Pinger: store, Events: events, EventStats: stats, Warehouse: warehouse, Kind: "postgres",
	}, func() { closeAll(); store.Close() }, nil
}

// SeedDemoUser creates the convenience demo account when configured
// (APP_DEMO_EMAIL / APP_DEMO_PASSWORD) and makes it a superadmin. It is
// idempotent: an existing account is left untouched. Do not set these
// variables in production.
func SeedDemoUser(auth *service.AuthService, rbac *service.RBACService, email, password string, logger *slog.Logger) {
	if email == "" || password == "" {
		return
	}
	user, _, err := auth.Register(context.Background(), email, "Demo User", password)
	switch {
	case errors.Is(err, repository.ErrConflict):
		existing, getErr := auth.UserByEmail(context.Background(), email)
		if getErr != nil {
			logger.Warn("demo user lookup failed", "err", getErr)
			return
		}
		user = existing
	case err != nil:
		logger.Warn("demo user seed failed", "err", err)
		return
	default:
		logger.Info("demo user seeded", "email", email)
	}
	if rbac != nil {
		if err := rbac.AssignRole(user.ID, "superadmin", service.PlatformDomain); err != nil {
			logger.Warn("demo user role grant failed", "err", err)
		}
	}
}

// compoundSink fans events out to every sink (local trail + warehouse).
type compoundSink []service.EventSink

func (sinks compoundSink) AddEvent(ctx context.Context, event, userID string) {
	for _, sink := range sinks {
		sink.AddEvent(ctx, event, userID)
	}
}
