// Command worker consumes background jobs from Redis via asynq when
// APP_REDIS_ADDR is set; otherwise it polls the local store for development.
// Independently of the job queue it runs the enrichment stream consumers
// (Kafka + NATS), the transactional outbox relay, and the idempotency-key
// pruning loop.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/your-org/go-template/internal/bootstrap"
	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/export"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/platform/queue"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/reliability"
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

	blobs := bootstrap.FileStore(ctx, cfg, logger)

	services := bootstrap.NewContainer(bootstrap.Deps{
		Auth: repos.Auth, Users: repos.Users, Jobs: repos.Jobs, Reports: repos.Reports,
		Orgs: repos.Orgs, Roles: repos.Roles, Audit: repos.Audit, Settings: repos.Settings, Notifications: repos.Notifications, EmailTokens: repos.EmailTokens, APIKeys: repos.APIKeys, Webhooks: repos.Webhooks, Tickets: repos.Tickets,
		Streams: repos.Streams,
		MFA:     repos.MFA, Tokens: repos.Tokens, Search: repos.Search, Invitations: repos.Invitations, Files: repos.Files,
		Blobs:    blobs,
		Breakers: breakers, Bulkheads: bulkheads,
		Events: repos.Events, EventStats: repos.EventStats, Observer: observ.NewJobObserver(),
		StreamsObserver: observ.NewStreamsObserver(),
	}, logger, cfg)

	if services.Files != nil {
		services.Jobs.WithExporter(export.NewService(repos.Users, repos.Auth, repos.Tickets, repos.Notifications, repos.Audit, repos.Files, blobs))
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

	// GDPR export ZIPs expire after export.Retention (docs/guides/data-export.md);
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

	// The email delivery log ages out on APP_EMAIL_RETENTION.
	if services.Email != nil {
		go emailPruneLoop(ctx, services.Email, cfg.EmailRetention, logger)
	}

	// Resumable uploads left unfinished past 24h abort here (their parts
	// cost storage money until they do).
	if services.Files != nil {
		go multipartPruneLoop(ctx, services.Files, logger)
	}

	// The audit trail ages out on APP_AUDIT_RETENTION (0 = forever) via the
	// audit_prune function — the only sanctioned delete path.
	go auditPruneLoop(ctx, repos.Audit, cfg.AuditRetention, logger)

	// Stream and relay metrics live in this process; expose them for
	// Prometheus when a metrics address is configured.
	closeMetrics := serveMetrics(cfg.MetricsAddr, logger)
	defer closeMetrics()

	// Job transitions flow to the api process over Redis pub/sub so browser
	// SSE streams see worker activity live.
	if cfg.RedisAddr != "" {
		bridge := realtime.NewRedisBridge(cfg.RedisAddr, realtime.NewHub(), logger)
		defer func() { _ = bridge.Close() }()
		services.Jobs.WithJobNotifier(func(job jobs.Job) {
			bridge.Publish(realtime.NewOrgEvent(realtime.TypeJobUpdated, job, job.OrgID))
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
func startStreams(ctx context.Context, services *bootstrap.Container, brokers *bootstrap.Streams, cfg config.Config, logger *slog.Logger) {
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

// runLocal is the single-process development path: poll the shared store.
func runLocal(ctx context.Context, services *bootstrap.Container, kind string, logger *slog.Logger) {
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
func runAsynq(ctx context.Context, services *bootstrap.Container, cfg config.Config, kind string, logger *slog.Logger) {
	opt := asynq.RedisClientOpt{Addr: cfg.RedisAddr}
	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency:    cfg.MaxConcurrency,
		RetryDelayFunc: asynq.DefaultRetryDelayFunc,
	})
	mux := queue.NewMux(services.Jobs, logger, services.Webhooks, services.Email)

	// The periodic report cron runs on every worker; slot-bucketed task ids
	// make concurrent enqueues deduplicate (see cmd/worker/cron.go).
	if cfg.ReportCron != "" {
		cronClient := asynq.NewClient(opt)
		defer func() { _ = cronClient.Close() }()
		go runReportCron(ctx, cronClient, cfg.ReportCron, logger)
		logger.Info("periodic report scheduled", "cron", cfg.ReportCron)
	}

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
