package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/reliability"
)

// JobQueue pushes jobs onto the distributed queue. A nil implementation
// keeps the single-process development path.
type JobQueue interface {
	Enqueue(ctx context.Context, job model.Job) error
}

const enrichBatchURL = "https://randomuser.me/api/?results=1"

// JobService implements enqueueing and local processing of background jobs.
// With a queue wired, producers publish to Redis and the worker process
// executes; without one, ProcessOne runs jobs in-process for development.
type JobService struct {
	store          JobsRepo
	reports        ReportsRepo
	queue          JobQueue
	observer       JobObserver
	events         EventSink
	logger         *slog.Logger
	client         *http.Client
	maxConcurrency int
}

// emit records an analytics event when a sink is configured.
func (s *JobService) emit(ctx context.Context, event string) {
	if s.events != nil {
		s.events.AddEvent(ctx, event)
	}
}

// NewRecord persists a queued job without publishing anything; the queue
// producer and the scheduler both start from here.
func (s *JobService) NewRecord(ctx context.Context, typ, payload string) (model.Job, error) {
	now := time.Now()
	job := model.Job{ID: newID(), Type: typ, Status: "queued", Payload: payload, CreatedAt: now, UpdatedAt: now}
	if err := s.store.SaveJob(ctx, job); err != nil {
		return model.Job{}, fmt.Errorf("save job: %w", err)
	}
	return job, nil
}

// Enqueue records the job and publishes it to the distributed queue. On a
// publish failure the record is marked failed so the UI reflects reality.
func (s *JobService) Enqueue(ctx context.Context, typ, payload string) (model.Job, error) {
	job, err := s.NewRecord(ctx, typ, payload)
	if err != nil {
		return model.Job{}, err
	}
	if s.observer != nil {
		s.observer.JobQueued(ctx)
	}
	s.emit(ctx, "job_queued")
	if s.queue == nil {
		return job, nil
	}
	if err := s.queue.Enqueue(ctx, job); err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		job.UpdatedAt = time.Now()
		_ = s.store.SaveJob(ctx, job)
		return job, fmt.Errorf("enqueue: %w", err)
	}
	return job, nil
}

// List returns all jobs newest first.
func (s *JobService) List(ctx context.Context) ([]model.Job, error) {
	return s.store.ListJobs(ctx)
}

// ProcessOne claims and runs the oldest queued job locally. With a
// distributed queue configured it is a no-op: another process owns
// execution.
func (s *JobService) ProcessOne(ctx context.Context) error {
	if s.queue != nil {
		return nil
	}
	pending, err := s.store.ListJobs(ctx)
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}
	for _, job := range pending {
		if job.Status != "queued" {
			continue
		}
		return s.RunJob(ctx, job.ID, func(ctx context.Context) error {
			return s.process(ctx, job)
		})
	}
	return nil
}

// RunJob syncs the operational record around the actual execution: running
// before, done or failed after, with events for the analytics trail.
func (s *JobService) RunJob(ctx context.Context, jobID string, run func(context.Context) error) error {
	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load job: %w", err)
	}
	job.Status = "running"
	job.UpdatedAt = time.Now()
	if err := s.store.SaveJob(ctx, job); err != nil {
		return fmt.Errorf("mark job running: %w", err)
	}
	start := time.Now()
	if err := run(ctx); err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		s.emit(ctx, "job_failed")
	} else {
		job.Status = "done"
		s.emit(ctx, "job_done")
	}
	job.UpdatedAt = time.Now()
	if err := s.store.SaveJob(ctx, job); err != nil {
		return fmt.Errorf("save job result: %w", err)
	}
	if s.observer != nil {
		s.observer.JobFinished(ctx, job.Type, job.Status, time.Since(start))
	}
	return nil
}

// EnrichBatch fetches count external profiles with bounded concurrency and
// retries; it is the body of user.enrich_batch.
func (s *JobService) EnrichBatch(ctx context.Context, count int) error {
	return reliability.RunBounded(ctx, count, s.maxConcurrency, func(ctx context.Context, i int) error {
		return reliability.Retry(ctx, 3, 100*time.Millisecond, func() error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, enrichBatchURL, nil)
			if err != nil {
				return err
			}
			resp, err := s.client.Do(req)
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode >= 500 {
				return fmt.Errorf("transient upstream status %d", resp.StatusCode)
			}
			if resp.StatusCode >= 400 {
				return reliability.PermanentError{Err: fmt.Errorf("permanent upstream status %d", resp.StatusCode)}
			}
			return s.store.AddEnrichment(ctx, model.EnrichmentProfile{
				UserID: newID(), Source: "randomuser.me", Payload: fmt.Sprintf("external-user-%d", i), CreatedAt: time.Now(),
			})
		})
	})
}

// GenerateReport builds a snapshot report and stores it; it is the body of
// user.report_generate.
func (s *JobService) GenerateReport(ctx context.Context) error {
	summary := s.reports.Analytics(ctx)
	now := time.Now()
	report := model.Report{ID: newID(), Status: "done", Result: formatSummary(summary), CreatedAt: now, UpdatedAt: now}
	return s.reports.SaveReport(ctx, report)
}

func (s *JobService) process(ctx context.Context, job model.Job) error {
	switch job.Type {
	case jobs.TypeEnrichBatch:
		var req struct {
			Count int `json:"count"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &req); err != nil {
			return fmt.Errorf("decode payload: %w", err)
		}
		if req.Count <= 0 || req.Count > 100 {
			req.Count = 10
		}
		return s.EnrichBatch(ctx, req.Count)
	case jobs.TypeReportGenerate:
		return s.GenerateReport(ctx)
	default:
		return fmt.Errorf("unknown job type %q", job.Type)
	}
}
