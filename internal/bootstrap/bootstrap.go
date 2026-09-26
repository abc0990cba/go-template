// Package bootstrap selects and opens the store adapter from configuration:
// PostgreSQL when APP_DATABASE_URL is set, the in-memory fake otherwise.
package bootstrap

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/your-org/go-template/internal/config"
	ch "github.com/your-org/go-template/internal/platform/clickhouse"
	"github.com/your-org/go-template/internal/platform/postgres"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

// Repos bundles the store ports; a single adapter fills them all.
type Repos struct {
	Auth      service.AuthRepo
	Users     service.UsersRepo
	Jobs      service.JobsRepo
	Reports   service.ReportsRepo
	Analytics service.AnalyticsRepo
	Pinger    service.Pinger
	// Events receives the analytics event trail; nil means the process does
	// not persist events (plain PostgreSQL mode).
	Events service.EventSink
	// EventStats reads aggregates from ClickHouse; nil without ClickHouse.
	EventStats service.EventStats
	Kind       string
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
	if cfg.ClickHouseAddr != "" {
		client, err := ch.Open(context.Background(), cfg.ClickHouseAddr, cfg.ClickHouseDB, cfg.ClickHouseUser, cfg.ClickHousePass, os.DirFS("migrations/clickhouse"))
		if err != nil {
			logger.Warn("clickhouse unavailable; events are not persisted", "err", err)
		} else {
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
			Pinger: store, Events: events, EventStats: stats, Kind: "memory",
		}, closeAll, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var opts []postgres.Option
	if cfg.OTLPEndpoint != "" {
		opts = append(opts, postgres.WithQueryTracing())
	}
	store, err := postgres.Open(ctx, cfg.DatabaseURL, os.DirFS("migrations/postgres"), opts...)
	if err != nil {
		return Repos{}, func() {}, err
	}
	logger.Info("postgres store ready", "migrations", "applied")
	return Repos{
		Auth: store, Users: store, Jobs: store, Reports: store, Analytics: store,
		Pinger: store, Events: events, EventStats: stats, Kind: "postgres",
	}, func() { closeAll(); store.Close() }, nil
}

// compoundSink fans events out to every sink (local trail + warehouse).
type compoundSink []service.EventSink

func (sinks compoundSink) AddEvent(ctx context.Context, event string) {
	for _, sink := range sinks {
		sink.AddEvent(ctx, event)
	}
}
