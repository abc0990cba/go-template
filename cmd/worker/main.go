// Command worker consumes background jobs. Today it polls the in-process
// memory store; the queue OpenSpec change moves it onto asynq with Redis.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

func main() {
	cfg, err := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	if err != nil {
		logger.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	store := repository.NewMemoryStore()
	services := service.NewContainer(store, logger, cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("worker started", "note", "in-memory development worker; replace adapter with asynq for shared Redis-backed queues")
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
