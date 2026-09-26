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
	"github.com/your-org/go-template/internal/platform/queue"
	"github.com/your-org/go-template/internal/service"
)

func main() {
	cfg, err := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	if err != nil {
		logger.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	repos, closeStore, err := bootstrap.Store(cfg, logger)
	if err != nil {
		logger.Error("open store", "err", err)
		os.Exit(1)
	}
	defer closeStore()

	services := service.NewContainer(repos.Auth, repos.Users, repos.Jobs, repos.Reports, repos.Analytics, nil, logger, cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.RedisAddr == "" {
		runLocal(ctx, services, repos.Kind, logger)
		return
	}
	runAsynq(ctx, services, cfg, repos.Kind, logger)
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
			if err := services.Jobs.ProcessOne(ctx); err != nil {
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
	mux := queue.NewMux(services.Jobs, logger)

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
