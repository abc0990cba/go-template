// Command seed-demo loads the demo dataset (users, identities, jobs,
// reports, enrichment profiles, and a backfilled event history) so the UI
// has realistic content to explore. It requires APP_DATABASE_URL.
//
//	make seed-demo          (or: go run ./cmd/seed-demo)
//	make seed-demo-reset    (or: go run ./cmd/seed-demo -reset)
package main

import (
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/your-org/go-template/internal/bootstrap"
	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/service"
)

func main() {
	reset := flag.Bool("reset", false, "delete existing demo data before seeding")
	flag.Parse()

	cfg, err := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err != nil {
		logger.Error("invalid configuration", "err", err)
		os.Exit(1)
	}
	if cfg.DatabaseURL == "" {
		logger.Error("seed-demo requires APP_DATABASE_URL (the in-memory store lives inside the api process; start the api with APP_DEMO_SEED=1 instead)")
		os.Exit(1)
	}

	repos, closeStore, err := bootstrap.Store(cfg, logger)
	if err != nil {
		logger.Error("open store", "err", err)
		os.Exit(1)
	}
	defer closeStore()

	services := service.NewContainer(service.Deps{
		Auth: repos.Auth, Users: repos.Users, Jobs: repos.Jobs, Reports: repos.Reports,
		Analytics: repos.Analytics, Orgs: repos.Orgs, Roles: repos.Roles, Audit: repos.Audit, Settings: repos.Settings, Notifications: repos.Notifications, EmailTokens: repos.EmailTokens, APIKeys: repos.APIKeys, Webhooks: repos.Webhooks,
		Events: repos.Events, EventStats: repos.EventStats, Observer: observ.NewJobObserver(),
	}, logger, cfg)

	started := time.Now()
	if *reset {
		if err := bootstrap.ResetAndSeedDemoData(services, repos, cfg, logger); err != nil {
			logger.Error("reset and seed demo data", "err", err)
			os.Exit(1)
		}
		logger.Info("demo data reset and reseeded", "duration", time.Since(started).Round(time.Millisecond))
		return
	}
	if err := bootstrap.SeedDemoDataIfMissing(services, repos, cfg, logger); err != nil {
		logger.Error("seed demo data", "err", err)
		os.Exit(1)
	}
	logger.Info("demo data ready", "duration", time.Since(started).Round(time.Millisecond))
}
