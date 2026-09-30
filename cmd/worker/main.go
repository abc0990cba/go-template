// Command worker consumes background jobs from Redis via asynq when
// APP_REDIS_ADDR is set; otherwise it polls the local store for development.
// Independently of the job queue it runs the enrichment stream consumers
// (Kafka + NATS), the transactional outbox relay, and the idempotency-key
// pruning loop.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/your-org/go-template/internal/bootstrap"
	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/platform/queue"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/reliability"
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

	// Continuous profiling: off until APP_PYROSCOPE_URL is set.
	defer observ.StartProfiler(cfg.PyroscopeURL, build)()

	shutdownTracing, err := observ.SetupTracing(ctx, cfg.OTLPEndpoint, cfg.OTLPSampleRatio, build)
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

	// Pool saturation is invisible to the default Go runtime metrics; the
	// custom collector exports pgxpool.Stat() (waiters, acquired share...).
	if repos.PoolStats != nil {
		prometheus.MustRegister(observ.NewPoolStatsCollector(repos.PoolStats))
	}

	breakers := reliability.NewBreakers(cfg.CBFailureThreshold, cfg.CBOpenTimeout)
	breakers.SetOnChange(func(name string, from, to reliability.CircuitState) {
		observ.RecordBreakerState(name, int(from), int(to))
	})

	// Bulkheads bound concurrent calls per outbound dependency; slot
	// occupancy and rejections land in Prometheus.
	bulkheads := reliability.NewBulkheads(cfg.BulkheadLimit, cfg.BulkheadAcquireTimeout)
	bulkheads.SetObservers(observ.RecordBulkheadInFlight, observ.RecordBulkheadRejected)

	blobs := bootstrap.FileStore(cfg, ctx, logger)

	services := service.NewContainer(service.Deps{
		Auth: repos.Auth, Users: repos.Users, Jobs: repos.Jobs, Reports: repos.Reports,
		Analytics: repos.Analytics, Orgs: repos.Orgs, Roles: repos.Roles, Audit: repos.Audit, Settings: repos.Settings, Notifications: repos.Notifications, EmailTokens: repos.EmailTokens, APIKeys: repos.APIKeys, Webhooks: repos.Webhooks, Tickets: repos.Tickets,
		Streams: repos.Streams,
		MFA:     repos.MFA, Tokens: repos.Tokens, Search: repos.Search, Invitations: repos.Invitations, Files: repos.Files,
		Blobs:    blobs,
		Breakers: breakers, Bulkheads: bulkheads,
		Events: repos.Events, EventStats: repos.EventStats, Observer: observ.NewJobObserver(),
		StreamsObserver: observ.NewStreamsObserver(),
	}, logger, cfg)

	if services.Files != nil {
		services.Jobs.WithExporter(service.NewExportService(repos.Users, repos.Auth, repos.Tickets, repos.Notifications, repos.Audit, repos.Files, blobs))
	}

	// Brokers are optional: each side that fails to open disables its
	// consumer with a warning instead of taking the worker down.
	brokers := bootstrap.Brokers(ctx, cfg, logger, breakers)
	defer brokers.Close()
	startStreams(ctx, services, brokers, cfg, logger)

	// Idempotency responses expire; the worker owns the sweep.
	if repos.Idempotency != nil {
		go pruneLoop(ctx, repos.Idempotency, logger)
	}

	// GDPR export ZIPs expire after ExportRetention (docs/data-export.md);
	// the builder exposes the sweep, this loop schedules it.
	if services.Exports != nil {
		go exportPruneLoop(ctx, services.Exports, logger)
	}

	// The exactly-once inbox doubles as the dedupe table; rows older than
	// the retention window can no longer be redelivered and are swept.
	go processedEventsPruneLoop(ctx, repos.Streams, cfg.RetentionProcessedEvents, logger)

	// Expired MFA login challenges and rotated-out refresh tokens are
	// swept here too.
	if services.MFA != nil {
		go mfaChallengePruneLoop(ctx, services.MFA, logger)
	}
	if services.Tokens != nil {
		go refreshTokenPruneLoop(ctx, services.Tokens, logger)
	}

	// Stream and relay metrics live in this process; expose them for
	// Prometheus when a metrics address is configured.
	closeMetrics := serveMetrics(cfg.MetricsAddr, logger)
	defer closeMetrics()

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

// newLogger picks text for humans and json for log shippers. Every line
// that runs inside a span carries trace_id/span_id (see observ.TraceHandler).
func newLogger(format string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(observ.NewTraceHandler(handler))
}

// startStreams launches the enrichment consumers and the outbox relay in
// supervised background goroutines; they stop when ctx is cancelled. A nil
// broker side simply starts nothing.
func startStreams(ctx context.Context, services *service.Container, brokers *bootstrap.Streams, cfg config.Config, logger *slog.Logger) {
	started := false
	if brokers.Kafka != nil {
		started = true
		go supervise(ctx, "kafka", logger, func(ctx context.Context) error {
			return brokers.Kafka.Consume(ctx, services.Streams.HandleProfileEvent)
		})
	}
	if brokers.NATS != nil {
		started = true
		go supervise(ctx, "nats", logger, func(ctx context.Context) error {
			return brokers.NATS.Consume(ctx, services.Streams.HandleContactEvent)
		})
	}
	if brokers.Publisher != nil {
		started = true
		services.Streams.WithRelayPublisher(brokers.Publisher)
		go relayLoop(ctx, services.Streams, brokers.Publisher, cfg, logger)
	}
	if started {
		logger.Info("streams started",
			"kafka", brokers.Kafka != nil, "nats", brokers.NATS != nil,
			"relay", brokers.Publisher != nil, "interval", cfg.OutboxInterval.String())
	}
}

// supervise restarts a consumer after fatal exits with exponential backoff
// until shutdown; transient problems never surface here because the consume
// loops handle them internally.
func supervise(ctx context.Context, name string, logger *slog.Logger, run func(context.Context) error) {
	backoff := 2 * time.Second
	for {
		err := run(ctx)
		if ctx.Err() != nil {
			return
		}
		logger.Error("stream consumer exited; restarting", "stream", name, "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

// relayLoop drains the transactional outbox on a fixed interval.
func relayLoop(ctx context.Context, streams *service.StreamsService, publisher service.EventPublisher, cfg config.Config, logger *slog.Logger) {
	relay := service.RelayConfig{Batch: cfg.OutboxBatch, LeaseSeconds: 60, MaxAttempts: cfg.OutboxMaxAttempts}
	ticker := time.NewTicker(cfg.OutboxInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := streams.RelayOutboxOnce(ctx, publisher, relay)
			if err != nil {
				logger.Warn("outbox relay pass failed", "published", n, "err", err)
			} else if n > 0 {
				logger.Debug("outbox relay published", "events", n)
			}
		}
	}
}

// pruneLoop deletes expired idempotency reservations on a fixed interval.
func pruneLoop(ctx context.Context, repo service.IdempotencyRepo, logger *slog.Logger) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := repo.PruneIdempotency(ctx); err != nil {
				logger.Debug("idempotency prune failed", "err", err)
			} else if n > 0 {
				observ.PrunedRows.WithLabelValues("idempotency_keys").Add(float64(n))
				logger.Debug("idempotency keys pruned", "count", n)
			}
		}
	}
}

// exportPruneLoop deletes expired GDPR export bundles hourly; the retention
// window lives in the export service (ExportRetention).
func exportPruneLoop(ctx context.Context, exports *service.ExportService, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := exports.Prune(ctx)
			if err != nil {
				logger.Debug("export prune failed", "err", err)
			} else if n > 0 {
				observ.PrunedRows.WithLabelValues("export_files").Add(float64(n))
				logger.Info("export bundles pruned", "count", n)
			}
		}
	}
}

// processedEventsPruneLoop bounds the exactly-once inbox; the retention
// window must stay longer than any redelivery delay the brokers produce.
func processedEventsPruneLoop(ctx context.Context, repo service.StreamsRepo, olderThan time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := repo.PruneProcessedEvents(ctx, olderThan)
			if err != nil {
				logger.Debug("processed events prune failed", "err", err)
			} else if n > 0 {
				observ.PrunedRows.WithLabelValues("processed_events").Add(float64(n))
				logger.Debug("processed events pruned", "count", n)
			}
		}
	}
}

// mfaChallengePruneLoop drops expired MFA login challenges every 10 minutes.
func mfaChallengePruneLoop(ctx context.Context, mfa *service.MFAService, logger *slog.Logger) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := mfa.PruneChallenges(ctx); err != nil {
				logger.Debug("mfa challenge prune failed", "err", err)
			}
		}
	}
}

// refreshTokenPruneLoop drops long-expired refresh tokens hourly.
func refreshTokenPruneLoop(ctx context.Context, tokens *service.TokenService, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := tokens.Prune(ctx); err != nil {
				logger.Debug("refresh token prune failed", "err", err)
			}
		}
	}
}

// serveMetrics exposes /metrics on its own listener; an empty address keeps
// the worker without a network listener, as compose expects by default.
func serveMetrics(addr string, logger *slog.Logger) func() {
	if addr == "" {
		return func() {}
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	logger.Info("metrics listening", "addr", addr)
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
		}
		select {
		case err := <-serveErr:
			logger.Debug("metrics server stopped with error", "err", err)
		default:
		}
	}
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
