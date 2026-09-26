// Package bootstrap selects and opens the store adapter from configuration:
// PostgreSQL when APP_DATABASE_URL is set, the in-memory fake otherwise.
package bootstrap

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/your-org/go-template/internal/config"
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
	Kind      string
}

// Store opens the configured adapter. The returned function releases it.
func Store(cfg config.Config, logger *slog.Logger) (Repos, func(), error) {
	if cfg.DatabaseURL == "" {
		store := repository.NewMemoryStore()
		return Repos{
			Auth: store, Users: store, Jobs: store, Reports: store, Analytics: store,
			Pinger: store, Kind: "memory",
		}, func() {}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := postgres.Open(ctx, cfg.DatabaseURL, os.DirFS("migrations/postgres"))
	if err != nil {
		return Repos{}, func() {}, err
	}
	logger.Info("postgres store ready", "migrations", "applied")
	return Repos{
		Auth: store, Users: store, Jobs: store, Reports: store, Analytics: store,
		Pinger: store, Kind: "postgres",
	}, store.Close, nil
}
