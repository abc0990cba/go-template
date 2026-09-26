// Command worker consumes background jobs from Redis via asynq when
// APP_REDIS_ADDR is set; otherwise it polls the local store for development.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"github.com/your-org/go-template/internal/bootstrap"
	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/platform/queue"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/service"
)

func main() {
	cfg, err := config.Load()
	logger := newLogger(cfg.LogFormat, cfg.LogLevel)
	if err != nil {
		logger.Error("invalid configuration", "err", err)
		os.Exit(1)
	}
	build := observ.Build{Version: version, Commit: commit, Date: date}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := observ.SetupTracing(ctx, cfg.OTLPEndpoint, build)
	if err != nil {
		logger.Error("setup tracing", "err", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTracing(context.Background()) }()

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

	// Job transitions flow to the api process over Redis pub/sub so browser
	// SSE streams see worker activity live.
	if cfg.RedisAddr != "" {
		bridge := realtime.NewRedisBridge(cfg.RedisAddr, realtime.NewHub(), logger)
		defer func() { _ = bridge.Close() }()
		services.Jobs.WithJobNotifier(func(job model.Job) {
			bridge.Publish(realtime.NewEvent(realtime.TypeJobUpdated, job))
		})
	}

	_ = stop
	if cfg.RedisAddr == "" {
		runLocal(ctx, services, repos.Kind, logger)
		return
	}
	runAsynq(ctx, services, cfg, repos.Kind, logger)
}

// Build metadata, stamped at link time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// newLogger picks text for humans and json for log shippers.
func newLogger(format string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	if format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

// runLocal is the single-process development path: poll the shared store.
func runLocal(ctx context.Context, services *service.Container, kind string, logger *slog.Logger) {
	logger.Info("worker started", "mode", "local-poll", "store", kind)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("worker stopped")
			return
		case <-ticker.C:
			if _, err := services.Jobs.ProcessOne(ctx); err != nil {
				logger.Error("job processing failed", "err", err)
			}
		}
	}
}

// runAsynq runs the distributed worker plus the periodic task scheduler.
func runAsynq(ctx context.Context, services *service.Container, cfg config.Config, kind string, logger *slog.Logger) {
	opt := asynq.RedisClientOpt{Addr: cfg.RedisAddr}
	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency:    cfg.MaxConcurrency,
		RetryDelayFunc: asynq.DefaultRetryDelayFunc,
	})
	mux := queue.NewMux(services.Jobs, logger, services.Webhooks)

	scheduler := asynq.NewScheduler(opt, nil)
	if cfg.ReportCron != "" {
		payload, err := jobs.Encode(jobs.ReportGenerate{PayloadVersion: 1})
		if err != nil {
			logger.Error("encode scheduled payload", "err", err)
			os.Exit(1)
		}
		if _, err := scheduler.Register(cfg.ReportCron, asynq.NewTask(jobs.TypeReportGenerate, payload)); err != nil {
			logger.Error("register periodic task", "cron", cfg.ReportCron, "err", err)
			os.Exit(1)
		}
		logger.Info("periodic report scheduled", "cron", cfg.ReportCron)
	}
	if err := scheduler.Start(); err != nil {
		logger.Error("start scheduler", "err", err)
		os.Exit(1)
	}
	defer scheduler.Shutdown()

	logger.Info("worker started", "mode", "asynq", "redis", cfg.RedisAddr, "concurrency", cfg.MaxConcurrency, "store", kind)
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Run(mux)
	}()

	select {
	case err := <-serveErr:
		logger.Error("worker failed", "err", err)
		os.Exit(1)
	case <-ctx.Done():
	}
	srv.Shutdown()
	logger.Info("worker stopped")
}
