// Package bootstrap selects and opens the store adapter from configuration:
// PostgreSQL when APP_DATABASE_URL is set, the in-memory fake otherwise.
package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/apikeys"
	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/files"
	"github.com/your-org/go-template/internal/invitations"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/mfa"
	"github.com/your-org/go-template/internal/notifications"
	ch "github.com/your-org/go-template/internal/platform/clickhouse"
	platformfilestore "github.com/your-org/go-template/internal/platform/filestore"
	"github.com/your-org/go-template/internal/platform/postgres"
	"github.com/your-org/go-template/internal/rbac"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/search"
	"github.com/your-org/go-template/internal/settings"
	"github.com/your-org/go-template/internal/streams"
	"github.com/your-org/go-template/internal/tickets"
	"github.com/your-org/go-template/internal/tokens"
	"github.com/your-org/go-template/internal/users"
	"github.com/your-org/go-template/internal/webhooks"
)

// Repos bundles the store ports; a single adapter fills them all.
type Repos struct {
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
	Idempotency   kernel.IdempotencyRepo
	MFA           mfa.Store
	Tokens        tokens.Store
	Search        search.Store
	Invitations   invitations.Store
	Files         files.Store
	Rules         rbac.RuleStore
	Pinger        kernel.Pinger
	// PoolStats snapshots pgxpool statistics for the Prometheus collector;
	// nil in memory mode (no pool to observe).
	PoolStats func() *pgxpool.Stat
	// Events receives the analytics event trail; nil means the process does
	// not persist events (plain PostgreSQL mode).
	Events analytics.Sink
	// EventStats reads aggregates from ClickHouse; nil without ClickHouse.
	EventStats analytics.Stats
	// Warehouse accepts historical event batches (the demo seed backfill);
	// nil when events cannot be written in bulk.
	Warehouse EventBackfiller
	Kind      string
}

// EventBackfiller writes a batch of historical events in one insert.
type EventBackfiller interface {
	InsertEvents(ctx context.Context, events []analytics.EventRecord) error
	// DeleteDemoEvents removes seed-generated events (demo reset).
	DeleteDemoEvents(ctx context.Context) error
}

// Store opens the configured adapters. The returned function releases them.
// ClickHouse is opened on top of the operational store when
// APP_CLICKHOUSE_ADDR is set; its failure to open is logged, not fatal —
// the app runs without the analytics warehouse.
func Store(cfg config.Config, logger *slog.Logger) (Repos, func(), error) {
	var (
		events   analytics.Sink
		stats    analytics.Stats
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
			Auth: store, Users: store, Jobs: store, Reports: store,
			Orgs: store, Roles: store, Audit: store, Settings: store, Notifications: store, EmailTokens: store, APIKeys: store, Webhooks: store, Tickets: store, Streams: store, Idempotency: store, Rules: store, MFA: store, Tokens: store, Search: store, Invitations: store, Files: store,
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
		Auth: store, Users: store, Jobs: store, Reports: store,
		Orgs: store, Roles: store, Audit: store, Settings: store, Notifications: store, EmailTokens: store, APIKeys: store, Webhooks: store, Tickets: store, Streams: store, Idempotency: store, Rules: store, MFA: store, Tokens: store, Search: store, Invitations: store, Files: store,
		Pinger: store, Events: events, EventStats: stats, Warehouse: warehouse, Kind: "postgres",
		PoolStats: store.Stats,
	}, func() { closeAll(); store.Close() }, nil
}

// SeedDemoUser creates the convenience demo account when configured
// (APP_DEMO_EMAIL / APP_DEMO_PASSWORD) and makes it a superadmin. It is
// idempotent: an existing account is left untouched. Do not set these
// variables in production.
func SeedDemoUser(authSvc *auth.Service, rbacSvc *rbac.Service, email, password string, logger *slog.Logger) {
	if email == "" || password == "" {
		return
	}
	user, _, err := authSvc.Register(context.Background(), email, "Demo User", password)
	switch {
	case errors.Is(err, kernel.ErrConflict):
		existing, getErr := authSvc.UserByEmail(context.Background(), email)
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
	if rbacSvc != nil {
		if err := rbacSvc.AssignRole(user.ID, "superadmin", rbac.PlatformDomain); err != nil {
			logger.Warn("demo user role grant failed", "err", err)
		}
	}
}

// compoundSink fans events out to every sink (local trail + warehouse).
type compoundSink []analytics.Sink

func (sinks compoundSink) AddEvent(ctx context.Context, event, userID string) {
	for _, sink := range sinks {
		sink.AddEvent(ctx, event, userID)
	}
}

// FileStore selects the blob adapter: S3/MinIO when configured (probe
// failure degrades to the in-process store with a warning), in-process
// otherwise. Both binaries share this so exports can be produced by the
// worker whatever the deployment mode.
func FileStore(cfg config.Config, ctx context.Context, logger *slog.Logger) files.FileStore {
	var blobs files.FileStore = platformfilestore.NewMemory()
	if cfg.S3Bucket == "" {
		return blobs
	}
	s3store, err := platformfilestore.NewS3(ctx, platformfilestore.S3Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
		AccessKeyID: cfg.S3AccessKeyID, SecretKey: cfg.S3SecretKey, UsePathStyle: cfg.S3UsePathStyle,
	})
	if err != nil {
		logger.Warn("s3 filestore unavailable; uploads stay in-process", "err", err)
		return blobs
	}
	logger.Info("s3 filestore ready", "bucket", cfg.S3Bucket)
	return s3store
}
