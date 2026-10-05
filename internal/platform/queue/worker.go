package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/observ"
	"github.com/your-org/go-template/internal/webhooks"
)

// NewMux builds the worker handler set on top of the job service. Handlers
// decode the versioned payloads, treat malformed ones as non-retryable, and
// sync the operational job record around execution. The webhooks and email
// services may be nil to skip their tasks.
func NewMux(jobService *jobs.Service, logger *slog.Logger, webhooks *webhooks.Service, emails *email.Service) *asynq.ServeMux {
	// The GDPR export handler lives in the jobs service via WithExporter.
	mux := asynq.NewServeMux()

	if emails != nil {
		mux.HandleFunc(jobs.TypeEmailDeliver, func(ctx context.Context, t *asynq.Task) error {
			var p jobs.EmailDeliver
			if err := json.Unmarshal(t.Payload(), &p); err != nil {
				return asynqPermanent(fmt.Errorf("decode %s payload: %w", t.Type(), err))
			}
			if p.MessageID == "" {
				return asynqPermanent(fmt.Errorf("email payload needs a message id"))
			}
			return emails.DeliverQueued(ctx, p.MessageID)
		})
	}

	if webhooks != nil {
		mux.HandleFunc(jobs.TypeWebhookDeliver, func(ctx context.Context, t *asynq.Task) error {
			var p jobs.WebhookDeliver
			if err := json.Unmarshal(t.Payload(), &p); err != nil {
				return asynqPermanent(fmt.Errorf("decode %s payload: %w", t.Type(), err))
			}
			return webhooks.DeliverQueued(ctx, p.EndpointID, p.DeliveryID)
		})
	}

	mux.HandleFunc(jobs.TypeEnrichBatch, func(ctx context.Context, t *asynq.Task) error {
		var p jobs.EnrichBatch
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return asynqPermanent(fmt.Errorf("decode %s payload: %w", t.Type(), err))
		}
		count, err := p.Validate()
		if err != nil {
			return asynqPermanent(err)
		}
		return jobService.RunJob(ctx, p.JobID, func(ctx context.Context) error {
			return jobService.EnrichBatch(ctx, count)
		})
	})

	mux.HandleFunc(jobs.TypeExportGenerate, func(ctx context.Context, t *asynq.Task) error {
		var p jobs.ExportGenerate
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return asynqPermanent(fmt.Errorf("decode %s payload: %w", t.Type(), err))
		}
		if p.UserID == "" {
			return asynqPermanent(fmt.Errorf("export payload needs a user id"))
		}
		return jobService.RunJob(ctx, p.JobID, func(ctx context.Context) error {
			return jobService.ProcessExport(ctx, p.UserID)
		})
	})

	mux.HandleFunc(jobs.TypeReportGenerate, func(ctx context.Context, t *asynq.Task) error {
		var p jobs.ReportGenerate
		if len(t.Payload()) > 0 {
			if err := json.Unmarshal(t.Payload(), &p); err != nil {
				return asynqPermanent(fmt.Errorf("decode %s payload: %w", t.Type(), err))
			}
		}
		// Scheduler-produced tasks carry no job id: create the record here,
		// once, before execution starts.
		jobID := p.JobID
		if jobID == "" {
			record, err := jobService.NewRecord(ctx, jobs.TypeReportGenerate, "{}", "")
			if err != nil {
				return err
			}
			jobID = record.ID
		}
		return jobService.RunJob(ctx, jobID, jobService.GenerateReport)
	})

	// The tracing middleware restores the producer's span context from the
	// task headers, so worker spans join the API trace. Task lifecycle logs
	// pass the context, so the lines carry trace_id/span_id too.
	mux.Use(func(h asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
			ctx, span := observ.SpanFromHeaders(ctx, t.Headers(), "asynq.process "+t.Type())
			defer span.End()
			id, _ := asynq.GetTaskID(ctx)
			logger.InfoContext(ctx, "task started", "type", t.Type(), "id", id)
			err := h.ProcessTask(ctx, t)
			if err != nil {
				logger.ErrorContext(ctx, "task failed", "type", t.Type(), "id", id, "err", err)
			} else {
				logger.InfoContext(ctx, "task done", "type", t.Type(), "id", id)
			}
			return err
		})
	})

	return mux
}

// asynqPermanent wraps a task failure so asynq does not retry it; retrying
// a malformed payload cannot succeed.
func asynqPermanent(err error) error {
	return fmt.Errorf("%w: %v", err, asynq.SkipRetry)
}
