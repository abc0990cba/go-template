package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/export"
	"github.com/your-org/go-template/internal/files"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/mfa"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/streams"
	"github.com/your-org/go-template/internal/tokens"
)

// relayLoop drains the transactional outbox on a fixed interval.
func relayLoop(ctx context.Context, svc *streams.Service, publisher streams.Publisher, cfg config.Config, logger *slog.Logger) {
	relay := streams.RelayConfig{Batch: cfg.OutboxBatch, LeaseSeconds: 60, MaxAttempts: cfg.OutboxMaxAttempts}
	ticker := time.NewTicker(cfg.OutboxInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := svc.RelayOutboxOnce(ctx, publisher, relay)
			if err != nil {
				logger.Warn("outbox relay pass failed", "published", n, "err", err)
			} else if n > 0 {
				logger.Debug("outbox relay published", "events", n)
			}
		}
	}
}

// pruneLoop deletes expired idempotency reservations on a fixed interval.
func pruneLoop(ctx context.Context, repo kernel.IdempotencyRepo, logger *slog.Logger) {
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
// window lives in the export service (export.Retention).
func exportPruneLoop(ctx context.Context, exports *export.Service, logger *slog.Logger) {
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
func processedEventsPruneLoop(ctx context.Context, repo streams.Store, olderThan time.Duration, logger *slog.Logger) {
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
func mfaChallengePruneLoop(ctx context.Context, svc *mfa.Service, logger *slog.Logger) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := svc.PruneChallenges(ctx); err != nil {
				logger.Debug("mfa challenge prune failed", "err", err)
			}
		}
	}
}

// refreshTokenPruneLoop drops long-expired refresh tokens hourly.
func refreshTokenPruneLoop(ctx context.Context, svc *tokens.Service, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := svc.Prune(ctx); err != nil {
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

// emailPruneLoop drops delivery-log rows past the retention window.
func emailPruneLoop(ctx context.Context, svc *email.Service, retention time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := svc.PruneMessages(ctx, time.Now().Add(-retention))
			if err != nil {
				logger.Debug("email message prune failed", "err", err)
			} else if n > 0 {
				observ.PrunedRows.WithLabelValues("email_messages").Add(float64(n))
				logger.Debug("email messages pruned", "count", n)
			}
		}
	}
}

// auditPruner is the retention surface of the audit store.
type auditPruner interface {
	PruneAuditLog(ctx context.Context, cutoff time.Time) (int64, error)
}

// auditPruneLoop trims the audit trail to APP_AUDIT_RETENTION through the
// audit_prune function (the only sanctioned delete path); 0 keeps entries
// forever.
func auditPruneLoop(ctx context.Context, repo auditPruner, retention time.Duration, logger *slog.Logger) {
	if retention <= 0 {
		logger.Info("audit retention disabled; entries are kept forever")
		return
	}
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := repo.PruneAuditLog(ctx, time.Now().Add(-retention))
			if err != nil {
				logger.Debug("audit prune failed", "err", err)
			} else if n > 0 {
				observ.PrunedRows.WithLabelValues("audit_log").Add(float64(n))
				logger.Info("audit entries pruned", "count", n)
			}
		}
	}
}

// multipartPruneLoop aborts resumable-upload sessions abandoned past the
// 24h budget; storage bills their uploaded parts until they complete or
// abort, so this loop is cost control.
func multipartPruneLoop(ctx context.Context, files *files.Service, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := files.AbortStaleMultiparts(ctx)
			if err != nil {
				logger.Debug("multipart sweep failed", "err", err)
			} else if n > 0 {
				observ.PrunedRows.WithLabelValues("multipart_uploads").Add(float64(n))
				logger.Info("stale multipart uploads aborted", "count", n)
			}
		}
	}
}
