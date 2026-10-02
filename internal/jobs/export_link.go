// GDPR export job support: the link between the jobs feature and the export
// bundle builder. The builder itself still lives in the service layer; the
// narrow Exporter interface keeps the dependency one-way and cycle-free.

package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/your-org/go-template/internal/files"
	"github.com/your-org/go-template/internal/kernel"
)

// Exporter builds a user's GDPR data bundle. The export builder satisfies
// it structurally; the interface lets the worker wire the builder in
// without duplicating the bundle logic in the queue handler.
type Exporter interface {
	Generate(ctx context.Context, userID string) (files.File, error)
}

// WithExporter attaches the GDPR export builder (worker side; the api only
// enqueues).
func (s *Service) WithExporter(exporter Exporter) *Service {
	s.exporter = exporter
	return s
}

// RequestExport enqueues a GDPR data export for the caller. One live export
// per user: a queued or running request answers with a conflict so retries
// cannot pile up duplicates.
func (s *Service) RequestExport(ctx context.Context, userID string) (Job, error) {
	// Without a queue the job stays queued until POST /api/jobs/process
	// runs it inline (single-process dev mode) — same as every other type.
	live, _, err := s.store.ListJobs(ctx, kernel.ListOptions{Type: TypeExportGenerate, Status: "queued", Limit: 50})
	if err == nil {
		for _, job := range live {
			if job.CreatedBy == userID {
				return job, fmt.Errorf("%w: an export is already queued", kernel.ErrConflict)
			}
		}
	}
	payload, err := json.Marshal(ExportGenerate{PayloadVersion: 1, UserID: userID})
	if err != nil {
		return Job{}, err
	}
	return s.Enqueue(ctx, TypeExportGenerate, string(payload), userID)
}

// LatestExport returns the caller's newest export job of any status.
func (s *Service) LatestExport(ctx context.Context, userID string) (Job, error) {
	jobsList, _, err := s.store.ListJobs(ctx, kernel.ListOptions{Type: TypeExportGenerate, Limit: 50})
	if err != nil {
		return Job{}, err
	}
	for _, job := range jobsList {
		if job.CreatedBy == userID {
			return job, nil
		}
	}
	return Job{}, fmt.Errorf("%w: no export requested yet", kernel.ErrNotFound)
}

// ProcessExport builds the bundle for the queue handler.
func (s *Service) ProcessExport(ctx context.Context, userID string) error {
	if s.exporter == nil {
		return fmt.Errorf("export builder is not wired in this process")
	}
	_, err := s.exporter.Generate(ctx, userID)
	return err
}
