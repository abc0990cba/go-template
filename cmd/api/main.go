// Command api runs the HTTP server: JSON API, the embedded SPA, the SSE
// stream, and health endpoints, with graceful shutdown on SIGINT/SIGTERM.
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

	"github.com/redis/go-redis/v9"

	platformcache "github.com/your-org/go-template/internal/platform/cache"

	"github.com/your-org/go-template/internal/bootstrap"
	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/httpapp"
	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/platform/mailer"
	asynqqueue "github.com/your-org/go-template/internal/platform/queue"
	"github.com/your-org/go-template/internal/platform/ratelimit"
	"github.com/your-org/go-template/internal/platform/rbac"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/reliability"
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

	// Realtime: the hub fans live events out to SSE subscribers; with Redis
	// present, the bridge also relays job transitions published by the
	// worker process.
	hub := realtime.NewHub()
	bridge := openRealtimeBridge(ctx, cfg, hub, logger)
	defer func() {
		if bridge != nil {
			_ = bridge.Close()
		}
	}()

	// Every analytics event flows through the fan sink: the store trail and
	// the ClickHouse buffer keep their behavior, and the hub publishes the
	// event to connected browsers.
	fan := realtime.NewFanSink(hub, nonNil(repos.Events)...)
	var events service.EventSink = fan

	jobQueue, closeQueue := openQueue(cfg)
	defer closeQueue()

	var webhookDispatcher service.WebhookDispatcher
	if jobQueue != nil {
		webhookDispatcher = asynqqueue.NewWebhookEnqueuer(cfg.RedisAddr)
	}

	// Distributed rate limiting shares one Redis budget across api
	// processes; single-process mode keeps token buckets in memory.
	redisClient := openRedis(cfg)

	// Read-through cache: shared via Redis when configured, in-process
	// otherwise. Users + expanded permissions are the hot reads.
	var appCache service.Cache
	if redisClient != nil {
		appCache = platformcache.NewRedis(redisClient)
	} else {
		appCache = platformcache.NewMemory()
	}

	// File store: S3/MinIO when configured (probe fails → warn + memory);
	// in-process blobs otherwise (dev convenience, resets on restart).
	blobs := bootstrap.FileStore(cfg, ctx, logger)

	// Circuit breakers wrap outbound calls; transitions land in Prometheus.
	breakers := reliability.NewBreakers(cfg.CBFailureThreshold, cfg.CBOpenTimeout)
	breakers.SetOnChange(func(name string, from, to reliability.CircuitState) {
		observ.RecordBreakerState(name, int(from), int(to))
	})

	// The policy engine guards every protected route; policies persist
	// through the same store and system roles are re-seeded idempotently.
	enforcer, err := rbac.NewEnforcer(repos.Rules)
	if err != nil {
		logger.Error("open rbac enforcer", "err", err)
		os.Exit(1)
	}

	sender := openMailer(cfg, logger)
	services := service.NewContainer(service.Deps{
		Auth: repos.Auth, Users: repos.Users, Jobs: repos.Jobs, Reports: repos.Reports,
		Analytics: repos.Analytics, Orgs: repos.Orgs, Roles: repos.Roles, Audit: repos.Audit,
		Settings: repos.Settings, Notifications: repos.Notifications, EmailTokens: repos.EmailTokens, APIKeys: repos.APIKeys,
		Webhooks: repos.Webhooks, Tickets: repos.Tickets, Streams: repos.Streams,
		MFA:               repos.MFA,
		Tokens:            repos.Tokens,
		Search:            repos.Search,
		Invitations:       repos.Invitations,
		Cache:             appCache,
		Files:             repos.Files,
		Blobs:             blobs,
		WebhookDispatcher: webhookDispatcher, Mailer: sender, Breakers: breakers,
		StreamsObserver: observ.NewStreamsObserver(),
		Realtime: func(n model.Notification) {
			hub.Publish(realtime.NewEvent(realtime.TypeNotification, n))
			if bridge != nil {
				bridge.Publish(realtime.NewEvent(realtime.TypeNotification, n))
			}
		},
		Events: events, EventStats: repos.EventStats, Queue: jobQueue,
		Observer: observ.NewJobObserver(), RBACEnforcer: enforcer,
	}, logger, cfg)
	// The export builder runs wherever jobs execute: the worker in queue
	// mode, the api's /api/jobs/process in single-process dev mode.
	services.Jobs.WithExporter(service.NewExportService(repos.Users, repos.Auth, repos.Tickets, repos.Notifications, repos.Audit, repos.Files, blobs))

	if err := services.RBAC.EnsureSystemRoles(ctx); err != nil {
		logger.Error("seed system roles", "err", err)
		os.Exit(1)
	}
	services.Jobs.WithJobNotifier(func(job model.Job) {
		hub.Publish(realtime.NewEvent(realtime.TypeJobUpdated, job))
		if bridge != nil {
			bridge.Publish(realtime.NewEvent(realtime.TypeJobUpdated, job))
		}
	})
	services.Tickets.WithNotifier(func(ticket model.Ticket) {
		hub.Publish(realtime.NewEvent(realtime.TypeTicketUpdated, ticket))
		if bridge != nil {
			bridge.Publish(realtime.NewEvent(realtime.TypeTicketUpdated, ticket))
		}
	})

	// The webhook fan-out receives every analytics event from the fan sink.
	fan.Register(realtime.WebhookSink{Webhooks: services.Webhooks})
	if webhookDispatcher == nil {
		services.Webhooks.UseInlineDispatcher()
	}
	bootstrap.SeedDemoUser(services.Auth, services.RBAC, cfg.DemoEmail, cfg.DemoPassword, logger)
	if cfg.DemoSeed {
		bootstrap.SeedDemoData(services, repos, cfg, logger)
	}

	// Broker pings join the readiness check only when the streams are on.
	// The probes are ping-only: the api never joins a consumer group — a
	// non-polling member would hold partitions away from the worker.
	var readyProbes []func(ctx context.Context) error
	var demoEnricher *bootstrap.DemoEnricher
	if len(cfg.KafkaBrokers) > 0 || cfg.NatsURL != "" {
		probes, closeProbes := bootstrap.Probes(ctx, cfg, logger)
		defer closeProbes()
		readyProbes = probes
		// Producer-only clients behind the admin console's "send test
		// event" buttons; nil when a broker is unreachable.
		demoEnricher = bootstrap.OpenDemoEnricher(ctx, cfg, logger)
		if demoEnricher != nil {
			defer demoEnricher.Close()
		}
	}

	router := httpapp.NewRouter(cfg, services, logger, repos.Pinger, build, hub, httpapp.Options{
		LimiterFactory: func(_ string, rps float64, burst int) ratelimit.Limiter {
			if redisClient != nil {
				return ratelimit.NewRedis(redisClient, time.Second, int(rps))
			}
			return ratelimit.NewMemory(rps, burst)
		},
		Idempotency:  repos.Idempotency,
		DemoEnricher: demoEnricher,
		ReadyProbes:  readyProbes,
	})

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
		// Long-lived Server-Sent Events streams keep a connection active
		// until the client hangs up, so Shutdown's grace period can expire
		// while one is open. Close what remains instead of failing.
		logger.Warn("graceful shutdown timed out; closing remaining connections (open SSE streams)", "err", err)
		_ = server.Close()
	}
}

// openRealtimeBridge subscribes to the worker's realtime channel when Redis
// is configured; nil otherwise (in-process events need no relay).
func openRealtimeBridge(ctx context.Context, cfg config.Config, hub *realtime.Hub, logger *slog.Logger) *realtime.RedisBridge {
	if cfg.RedisAddr == "" {
		return nil
	}
	bridge := realtime.NewRedisBridge(cfg.RedisAddr, hub, logger)
	go bridge.Run(ctx)
	return bridge
}

// nonNil filters nil sinks so the fan sink only wraps real ones.
func nonNil(sinks ...service.EventSink) []service.EventSink {
	out := make([]service.EventSink, 0, len(sinks))
	for _, sink := range sinks {
		if sink != nil {
			out = append(out, sink)
		}
	}
	return out
}

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

// openMailer picks the SMTP transport when configured, the logging
// fallback otherwise.
func openMailer(cfg config.Config, logger *slog.Logger) mailer.Sender {
	mailCfg := mailer.Config{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort,
		Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, From: cfg.SMTPFrom,
	}
	if !mailCfg.Enabled() {
		return mailer.NewLogMailer(logger)
	}
	return mailer.NewSMTP(mailCfg, logger)
}

// openRedis connects the shared limiter client when Redis is configured;
// nil keeps rate limiting in-process.
func openRedis(cfg config.Config) *redis.Client {
	if cfg.RedisAddr == "" {
		return nil
	}
	return redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
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
