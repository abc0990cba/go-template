// Package queue adapts asynq (Redis-backed) to the service JobQueue port:
// task ids mirror job ids, which makes enqueueing idempotent, and options
// bound retries and run time per task.
package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/observ"
)

// Queue is the asynq-backed producer.
type Queue struct {
	client *asynq.Client
}

// New opens the asynq client against a Redis address.
func New(redisAddr string) *Queue {
	return &Queue{client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})}
}

// Enqueue publishes the job as a task. The asynq task id equals the job id,
// so a retried producer request cannot create a duplicate job.
func (q *Queue) Enqueue(ctx context.Context, job model.Job) error {
	var (
		payload any
		opts    []asynq.Option
	)
	switch job.Type {
	case jobs.TypeEnrichBatch:
		var p struct {
			Count int `json:"count"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
			return fmt.Errorf("decode job payload: %w", err)
		}
		payload = jobs.EnrichBatch{PayloadVersion: 1, JobID: job.ID, Count: p.Count}
		opts = append(opts, asynq.Timeout(5*time.Minute))
	case jobs.TypeReportGenerate:
		payload = jobs.ReportGenerate{PayloadVersion: 1, JobID: job.ID}
		opts = append(opts, asynq.Timeout(time.Minute))
	case jobs.TypeExportGenerate:
		var p struct {
			UserID string `json:"user_id"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
			return fmt.Errorf("decode job payload: %w", err)
		}
		payload = jobs.ExportGenerate{PayloadVersion: 1, JobID: job.ID, UserID: p.UserID}
		opts = append(opts, asynq.Timeout(10*time.Minute))
	default:
		return fmt.Errorf("unknown job type %q", job.Type)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode task payload: %w", err)
	}
	headers := map[string]string{}
	observ.InjectHeaders(ctx, headers)
	opts = append(opts,
		asynq.TaskID(job.ID),
		asynq.MaxRetry(5),
		asynq.Queue("default"),
	)
	task := asynq.NewTaskWithHeaders(job.Type, body, headers)
	if _, err := q.client.EnqueueContext(ctx, task, opts...); err != nil {
		return fmt.Errorf("enqueue task: %w", err)
	}
	return nil
}

// Close releases the client connection.
func (q *Queue) Close() error { return q.client.Close() }

// WebhookEnqueuer pushes webhook deliveries onto the asynq queue; it
// implements the service.WebhookDispatcher port for multi-process mode.
type WebhookEnqueuer struct {
	client *asynq.Client
}

// NewWebhookEnqueuer opens the producer against a Redis address.
func NewWebhookEnqueuer(redisAddr string) *WebhookEnqueuer {
	return &WebhookEnqueuer{client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})}
}

// Deliver enqueues the delivery for the worker process.
func (w *WebhookEnqueuer) Deliver(ctx context.Context, endpoint model.WebhookEndpoint, delivery model.WebhookDelivery, _ []byte) {
	body, err := json.Marshal(jobs.WebhookDeliver{
		PayloadVersion: 1, EndpointID: endpoint.ID, DeliveryID: delivery.ID,
	})
	if err != nil {
		return
	}
	headers := map[string]string{}
	observ.InjectHeaders(ctx, headers)
	task := asynq.NewTaskWithHeaders(jobs.TypeWebhookDeliver, body, headers)
	_, _ = w.client.EnqueueContext(ctx, task,
		asynq.TaskID(delivery.ID), asynq.MaxRetry(3), asynq.Timeout(30*time.Second), asynq.Queue("default"))
}
