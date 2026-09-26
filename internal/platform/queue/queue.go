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
	default:
		return fmt.Errorf("unknown job type %q", job.Type)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode task payload: %w", err)
	}
	opts = append(opts,
		asynq.TaskID(job.ID),
		asynq.MaxRetry(5),
		asynq.Queue("default"),
	)
	if _, err := q.client.EnqueueContext(ctx, asynq.NewTask(job.Type, body), opts...); err != nil {
		return fmt.Errorf("enqueue task: %w", err)
	}
	return nil
}

// Close releases the client connection.
func (q *Queue) Close() error { return q.client.Close() }
