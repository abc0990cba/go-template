// Command api runs the HTTP server: JSON API, server-rendered UI, and health
// endpoints, with graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/your-org/go-template/internal/bootstrap"
	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/httpapp"
	"github.com/your-org/go-template/internal/observ"
	asynqqueue "github.com/your-org/go-template/internal/platform/queue"
	"github.com/your-org/go-template/internal/service"
)

// Build metadata, stamped at link time:
//
//	go build -ldflags "-X main.version=1.0.0 -X main.commit=$(git rev-parse --short HEAD)"
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "GET /health/live on the local port and exit")
	flag.Parse()
	if *healthcheck {
		os.Exit(runHealthcheck())
	}

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

	jobQueue, closeQueue := openQueue(cfg)
	defer closeQueue()

	services := service.NewContainer(repos.Auth, repos.Users, repos.Jobs, repos.Reports, repos.Analytics, repos.Events, repos.EventStats, jobQueue, observ.NewJobObserver(), logger, cfg)
	bootstrap.SeedDemoUser(services.Auth, cfg.DemoEmail, cfg.DemoPassword, logger)
	router := httpapp.NewRouter(cfg, services, logger, repos.Pinger, build)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("api listening", "addr", cfg.HTTPAddr, "log_level", cfg.LogLevel.String(), "store", repos.Kind)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		logger.Error("api failed", "err", err)
		os.Exit(1)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	logger.Info("shutting down api")
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
}

// newLogger picks text for humans and json for log shippers.
// runHealthcheck is the container HEALTHCHECK probe; distroless images have
// no shell or curl, so the binary checks itself.
func runHealthcheck() int {
	addr := os.Getenv("APP_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1%s/health/live", addr))
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}

func newLogger(format string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	if format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

// openQueue returns the asynq-backed job queue, or nil in single-process
// development mode where jobs run inline.
func openQueue(cfg config.Config) (service.JobQueue, func()) {
	if cfg.RedisAddr == "" {
		return nil, func() {}
	}
	queue := asynqqueue.New(cfg.RedisAddr)
	return queue, func() { _ = queue.Close() }
}
