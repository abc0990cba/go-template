package main

// Process wiring for the optional infrastructure the api binary needs:
// the realtime Redis bridge, the logger stack, mail, Redis, the breached-
// password gate, and the asynq queue. Everything here degrades to a safe
// zero value when its environment is unset, matching the config package's
// optional-infrastructure rule.

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/platform/hibp"
	"github.com/your-org/go-template/internal/platform/mailer"
	asynqqueue "github.com/your-org/go-template/internal/platform/queue"
	"github.com/your-org/go-template/internal/realtime"

	"github.com/your-org/go-template/internal/config"
)

func openRealtimeBridge(ctx context.Context, cfg config.Config, hub *realtime.Hub, logger *slog.Logger) *realtime.RedisBridge {
	if cfg.RedisAddr == "" {
		return nil
	}
	bridge := realtime.NewRedisBridge(cfg.RedisAddr, hub, logger)
	go bridge.Run(ctx)
	return bridge
}

// nonNil filters nil sinks so the fan sink only wraps real ones.
func nonNil(sinks ...analytics.Sink) []analytics.Sink {
	out := make([]analytics.Sink, 0, len(sinks))
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
	// #nosec G704 -- addr is the operator-configured listen address and the
	// target is loopback by construction: the container probing itself.
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
	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	// Scrub PII (credentials in attribute values, bare email addresses)
	// before anything else, then stamp trace_id/span_id on every line that
	// runs inside a span, so a log line jumps to its Jaeger trace (and back
	// via the request.id attribute the access log sets on the span).
	return slog.New(observ.NewTraceHandler(observ.NewRedactHandler(handler)))
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
// nil keeps rate limiting in-process. Commands get client spans (redisotel)
// so cache and limiter round-trips join the request trace.
func openRedis(cfg config.Config, logger *slog.Logger) *redis.Client {
	if cfg.RedisAddr == "" {
		return nil
	}
	client := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := redisotel.InstrumentTracing(client); err != nil {
		logger.Debug("redis tracing unavailable", "err", err)
	}
	return client
}

// openQueue returns the asynq-backed job queue, or nil in single-process
// development mode where jobs run inline.
// openPwnedGate builds the breached-password gate for the configured mode.
// nil is never returned in warn/enforce modes; the wrapping keeps the mode
// policy out of the auth and email services.
func openPwnedGate(cfg config.Config, logger *slog.Logger) kernel.PwnedChecker {
	client := hibp.New(cfg.HIBPURL, nil)
	return func(ctx context.Context, password string) (bool, error) {
		compromised, err := client.Compromised(ctx, password)
		if err != nil {
			// Availability over strictness: an unreachable corpus must not
			// lock the world out of registering.
			logger.WarnContext(ctx, "breached-password check unavailable; failing open", "err", err)
			return false, nil
		}
		if compromised && cfg.HIBPMode == "warn" {
			logger.WarnContext(ctx, "password matches a known breach; allowing in warn mode")
			return false, nil
		}
		return compromised, nil
	}
}

func openQueue(cfg config.Config) (jobs.Queue, func()) {
	if cfg.RedisAddr == "" {
		return nil, func() {}
	}
	queue := asynqqueue.New(cfg.RedisAddr)
	return queue, func() { _ = queue.Close() }
}
