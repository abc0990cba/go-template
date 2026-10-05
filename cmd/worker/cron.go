package main

// The periodic report cron. Every worker runs this loop, but enqueues are
// safe under concurrency by construction: each fire enqueues with a task id
// derived from the scheduled minute, and asynq rejects a second task with
// the same id — so N workers produce exactly one report job per slot, with
// no leader election and no failover window. (The former per-process
// asynq.Scheduler enqueued once per worker, silently duplicating the job.)

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"

	"github.com/your-org/go-template/internal/jobs"
)

// reportTaskID returns the dedup key for one cron slot: the cron spec is
// minute-granular, so bucketing the fire time to the minute makes every
// worker that fires for that slot compute the same id.
func reportTaskID(fire time.Time) string {
	return "report-generate:" + fire.UTC().Truncate(time.Minute).Format(time.RFC3339)
}

// runReportCron enqueues the report task on the configured cron schedule
// until ctx is done. It runs on every worker; the task id deduplicates the
// fire across them. A nil client (no Redis) is a no-op.
func runReportCron(ctx context.Context, client *asynq.Client, spec string, logger *slog.Logger) {
	if client == nil || strings.TrimSpace(spec) == "" {
		return
	}
	schedule, err := cron.ParseStandard(spec)
	if err != nil {
		logger.WarnContext(ctx, "invalid report cron spec; scheduled reports disabled", "cron", spec, "err", err)
		return
	}
	payload, err := jobs.Encode(jobs.ReportGenerate{PayloadVersion: 1})
	if err != nil {
		logger.WarnContext(ctx, "encode report payload failed; scheduled reports disabled", "err", err)
		return
	}

	for {
		next := schedule.Next(time.Now())
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		task := asynq.NewTask(jobs.TypeReportGenerate, payload, asynq.TaskID(reportTaskID(next)))
		if _, err := client.EnqueueContext(ctx, task, asynq.Queue("default")); err != nil {
			if errors.Is(err, asynq.ErrTaskIDConflict) {
				// Another worker won this slot — exactly the dedup working.
				logger.DebugContext(ctx, "report slot already enqueued by another worker", "slot", reportTaskID(next))
				continue
			}
			if ctx.Err() != nil {
				return
			}
			logger.WarnContext(ctx, "enqueue scheduled report failed", "err", err)
		} else {
			logger.InfoContext(ctx, "scheduled report enqueued", "slot", reportTaskID(next))
		}
	}
}
