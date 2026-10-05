// Command api runs the HTTP server: JSON API, the embedded SPA, the SSE
// stream, and health endpoints, with graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/cache"
	"github.com/your-org/go-template/internal/notifications"
	platformcache "github.com/your-org/go-template/internal/platform/cache"

	"github.com/your-org/go-template/internal/bootstrap"
	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/export"
	"github.com/your-org/go-template/internal/httpapp"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/observ"
	asynqqueue "github.com/your-org/go-template/internal/platform/queue"
	"github.com/your-org/go-template/internal/platform/ratelimit"
	"github.com/your-org/go-template/internal/platform/rbac"
	platformrevocations "github.com/your-org/go-template/internal/platform/revocations"
	"github.com/your-org/go-template/internal/realtime"
	"github.com/your-org/go-template/internal/reliability"
	"github.com/your-org/go-template/internal/tickets"
	"github.com/your-org/go-template/internal/tokens"
	"github.com/your-org/go-template/internal/webhooks"
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
	var events analytics.Sink = fan

	jobQueue, closeQueue := openQueue(cfg)
	defer closeQueue()

	var webhookDispatcher webhooks.Dispatcher
	if jobQueue != nil {
		webhookDispatcher = asynqqueue.NewWebhookEnqueuer(cfg.RedisAddr)
	}

	// JWT key rotation: parse the keyset fail-fast so a malformed secret
	// mount kills the deploy, not the login flow hours later.
	var jwtKeys *tokens.KeySet
	if len(cfg.JWTKeySetData) > 0 {
		parsed, err := tokens.ParseKeySet(cfg.JWTKeySetData, cfg.JWTSecret)
		if err != nil {
			logger.Error("invalid JWT keyset", "err", err)
			os.Exit(1)
		}
		jwtKeys = parsed
	}

	// Breached-password checks (APP_HIBP_MODE): the gate fails open when
	// the range API is unreachable, and warn mode logs but allows.
	var pwned kernel.PwnedChecker
	if cfg.HIBPMode != "off" {
		pwned = openPwnedGate(cfg, logger)
	}

	// Queued email (APP_EMAIL_MODE=queue): recorded messages go to the
	// worker for delivery; sync mode sends inline and still logs outcomes.
	var emailDispatcher email.Dispatcher
	if cfg.EmailMode == "queue" && jobQueue != nil {
		enqueuer := asynqqueue.NewEmailEnqueuer(cfg.RedisAddr)
		defer func() { _ = enqueuer.Close() }()
		emailDispatcher = enqueuer
	}

	// Distributed rate limiting shares one Redis budget across api
	// processes; single-process mode keeps token buckets in memory.
	redisClient := openRedis(cfg, logger)

	// Access-token denylist: shared via Redis so every replica honors a
	// revocation; in-process otherwise (single-replica deployments).
	var jwtRevocations tokens.Revocations
	if redisClient != nil {
		jwtRevocations = platformrevocations.New(redisClient)
	} else {
		jwtRevocations = tokens.NewMemoryRevocations()
	}

	// Read-through cache: shared via Redis when configured, in-process
	// otherwise. Users + expanded permissions are the hot reads.
	var appCache cache.Cache
	if redisClient != nil {
		appCache = platformcache.NewRedis(redisClient)
	} else {
		appCache = platformcache.NewMemory()
	}

	// File store: S3/MinIO when configured (probe fails → warn + memory);
	// in-process blobs otherwise (dev convenience, resets on restart).
	blobs := bootstrap.FileStore(ctx, cfg, logger)

	// Circuit breakers wrap outbound calls; transitions land in Prometheus.
	breakers := reliability.NewBreakers(cfg.CBFailureThreshold, cfg.CBOpenTimeout)
	breakers.SetOnChange(func(name string, from, to reliability.CircuitState) {
		observ.RecordBreakerState(name, int(from), int(to))
	})

	// Bulkheads bound concurrent calls per outbound dependency; slot
	// occupancy and rejections land in Prometheus.
	bulkheads := reliability.NewBulkheads(cfg.BulkheadLimit, cfg.BulkheadAcquireTimeout)
	bulkheads.SetObservers(observ.RecordBulkheadInFlight, observ.RecordBulkheadRejected)

	// The policy engine guards every protected route; policies persist
	// through the same store and system roles are re-seeded idempotently.
	enforcer, err := rbac.NewEnforcer(repos.Rules)
	if err != nil {
		logger.Error("open rbac enforcer", "err", err)
		os.Exit(1)
	}

	sender := openMailer(cfg, logger)
	services := bootstrap.NewContainer(bootstrap.Deps{
		Auth: repos.Auth, Users: repos.Users, Jobs: repos.Jobs, Reports: repos.Reports,
		Orgs: repos.Orgs, Roles: repos.Roles, Audit: repos.Audit,
		Settings: repos.Settings, Notifications: repos.Notifications, EmailTokens: repos.EmailTokens,
		EmailMessages: repos.EmailMessages, EmailDispatcher: emailDispatcher, LoginGuard: repos.LoginGuard, Pwned: pwned, JWTKeys: jwtKeys, JWTRevocations: jwtRevocations, APIKeys: repos.APIKeys,
		Webhooks: repos.Webhooks, Tickets: repos.Tickets, Streams: repos.Streams,
		MFA:               repos.MFA,
		Tokens:            repos.Tokens,
		Search:            repos.Search,
		Invitations:       repos.Invitations,
		Cache:             appCache,
		Files:             repos.Files,
		Blobs:             blobs,
		WebhookDispatcher: webhookDispatcher, Mailer: sender, Breakers: breakers, Bulkheads: bulkheads,
		StreamsObserver: observ.NewStreamsObserver(),
		Realtime: func(n notifications.Notification) {
			// Personal: only the target user's connections receive it.
			hub.Publish(realtime.NewUserEvent(realtime.TypeNotification, n, n.UserID))
			if bridge != nil {
				bridge.Publish(realtime.NewUserEvent(realtime.TypeNotification, n, n.UserID))
			}
		},
		Events: events, EventStats: repos.EventStats, Queue: jobQueue,
		Observer: observ.NewJobObserver(), RBACEnforcer: enforcer,
	}, logger, cfg)
	// The export builder runs wherever jobs execute: the worker in queue
	// mode, the api's /api/jobs/process in single-process dev mode.
	services.Jobs.WithExporter(export.NewService(repos.Users, repos.Auth, repos.Tickets, repos.Notifications, repos.Audit, repos.Files, blobs))

	// Activity events ride to the actor's orgs (admins see everything);
	// resolved per event, cached with the RBAC service's org cache.
	fan.WithOrgResolver(func(ctx context.Context, userID string) []string {
		memberships, err := services.RBAC.OrgsForUser(ctx, userID)
		if err != nil {
			return nil
		}
		ids := make([]string, 0, len(memberships))
		for _, org := range memberships {
			ids = append(ids, org.ID)
		}
		return ids
	})

	if err := services.RBAC.EnsureSystemRoles(ctx); err != nil {
		logger.Error("seed system roles", "err", err)
		os.Exit(1)
	}
	services.Jobs.WithJobNotifier(func(job jobs.Job) {
		// Org-scoped: members of the job's org (and platform admins) follow it.
		hub.Publish(realtime.NewOrgEvent(realtime.TypeJobUpdated, job, job.OrgID))
		if bridge != nil {
			bridge.Publish(realtime.NewOrgEvent(realtime.TypeJobUpdated, job, job.OrgID))
		}
	})
	services.Tickets.WithNotifier(func(ticket tickets.Ticket) {
		// Org-scoped: ticket content never leaves its organization.
		hub.Publish(realtime.NewOrgEvent(realtime.TypeTicketUpdated, ticket, ticket.OrgID))
		if bridge != nil {
			bridge.Publish(realtime.NewOrgEvent(realtime.TypeTicketUpdated, ticket, ticket.OrgID))
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
