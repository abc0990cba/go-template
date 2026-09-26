package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/reliability"
)

const enrichBatchURL = "https://randomuser.me/api/?results=1"

// JobService implements enqueueing and local processing of background
// jobs; the queue OpenSpec change moves execution onto asynq.
type JobService struct {
	store          JobsRepo
	logger         *slog.Logger
	client         *http.Client
	maxConcurrency int
}

// Enqueue persists a queued job of the given type with a raw JSON payload.
func (s *JobService) Enqueue(ctx context.Context, typ, payload string) (model.Job, error) {
	now := time.Now()
	job := model.Job{ID: newID(), Type: typ, Status: "queued", Payload: payload, CreatedAt: now, UpdatedAt: now}
	if err := s.store.SaveJob(ctx, job); err != nil {
		return model.Job{}, fmt.Errorf("save job: %w", err)
	}
	s.store.AddEvent(ctx, "job_queued")
	return job, nil
}

// List returns all jobs newest first.
func (s *JobService) List(ctx context.Context) ([]model.Job, error) {
	return s.store.ListJobs(ctx)
}

// ProcessOne claims and runs the oldest queued job, if any. An empty queue is
// not an error; only store failures are.
func (s *JobService) ProcessOne(ctx context.Context) error {
	jobs, err := s.store.ListJobs(ctx)
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}
	for _, job := range jobs {
		if job.Status != "queued" {
			continue
		}
		return s.run(ctx, job)
	}
	return nil
}

func (s *JobService) run(ctx context.Context, job model.Job) error {
	job.Status = "running"
	job.UpdatedAt = time.Now()
	if err := s.store.SaveJob(ctx, job); err != nil {
		return fmt.Errorf("mark job running: %w", err)
	}
	if err := s.process(ctx, job); err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		s.store.AddEvent(ctx, "job_failed")
	} else {
		job.Status = "done"
		s.store.AddEvent(ctx, "job_done")
	}
	job.UpdatedAt = time.Now()
	if err := s.store.SaveJob(ctx, job); err != nil {
		return fmt.Errorf("save job result: %w", err)
	}
	return nil
}

func (s *JobService) process(ctx context.Context, job model.Job) error {
	switch job.Type {
	case "user.enrich_batch":
		var req struct {
			Count int `json:"count"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &req); err != nil {
			return fmt.Errorf("decode payload: %w", err)
		}
		if req.Count <= 0 || req.Count > 100 {
			req.Count = 10
		}
		return s.enrichUsers(ctx, req.Count)
	case "user.report_generate":
		return nil
	default:
		return fmt.Errorf("unknown job type %q", job.Type)
	}
}

func (s *JobService) enrichUsers(ctx context.Context, count int) error {
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
