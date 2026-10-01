// Job and enrichment persistence for the in-memory store: an upsert map for
// jobs and an append-only sequence-numbered slice for enrichment profiles
// (the seq mirrors the Postgres column and is the keyset tiebreaker).
package repository

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
)

// SaveJob upserts a job by id.
func (s *MemoryStore) SaveJob(ctx context.Context, job jobs.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	return nil
}

// GetJob returns a job by id.
func (s *MemoryStore) GetJob(ctx context.Context, id string) (jobs.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return jobs.Job{}, kernel.ErrNotFound
	}
	return job, nil
}

// listJobsLocked returns the jobs matching opts, newest first with the id as
// tiebreaker, and the next-page cursor (empty on the last page). Caller holds
// the lock.
func (s *MemoryStore) listJobsLocked(opts kernel.ListOptions) ([]jobs.Job, string) {
	matching := make([]jobs.Job, 0, len(s.jobs))
	query := strings.ToLower(opts.Query)
	for _, job := range s.jobs {
		if opts.Status != "" && job.Status != opts.Status {
			continue
		}
		if opts.Type != "" && job.Type != opts.Type {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(job.Type+" "+job.Error), query) {
			continue
		}
		matching = append(matching, job)
	}
	sort.Slice(matching, func(i, j int) bool {
		if matching[i].CreatedAt.Equal(matching[j].CreatedAt) {
			return matching[i].ID < matching[j].ID
		}
		return matching[i].CreatedAt.After(matching[j].CreatedAt)
	})
	rows, next := pageSlice(matching, func(job jobs.Job) string { return job.ID }, opts)
	return rows, next
}

// ListJobs returns one page of jobs plus the next-page cursor.
func (s *MemoryStore) ListJobs(ctx context.Context, opts kernel.ListOptions) ([]jobs.Job, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, next := s.listJobsLocked(opts)
	return rows, next, nil
}

// enrichmentRow pairs a stored profile with its insertion sequence, the
// keyset tiebreaker mirroring the Postgres seq column.
type enrichmentRow struct {
	profile jobs.EnrichmentProfile
	seq     int64
}

// AddEnrichment appends an enrichment profile record with a monotonic
// sequence used as the keyset tiebreaker.
func (s *MemoryStore) AddEnrichment(ctx context.Context, profile jobs.EnrichmentProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enrichSeq++
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = time.Now()
	}
	s.enrichments = append(s.enrichments, enrichmentRow{profile: profile, seq: s.enrichSeq})
	return nil
}

// ListEnrichments returns one page of enrichment profiles newest first. The
// cursor is "<created_at unix nano>:<sequence>", shared with the Postgres
// adapter so both stores speak the same cursor dialect.
func (s *MemoryStore) ListEnrichments(ctx context.Context, opts kernel.ListOptions) ([]jobs.EnrichmentProfile, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := make([]enrichmentRow, 0, len(s.enrichments))
	query := strings.ToLower(opts.Query)
	for _, row := range s.enrichments {
		if opts.UserID != "" && row.profile.UserID != opts.UserID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(row.profile.Source+" "+row.profile.Payload), query) {
			continue
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].profile.CreatedAt.After(rows[j].profile.CreatedAt) })
	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	start := 0
	if opts.Mode == "cursor" || opts.Mode == "keyset" {
		if afterAt, afterSeq, ok := kernel.SplitEnrichmentCursor(opts.After); ok {
			for i, row := range rows {
				createdAt, seq := row.profile.CreatedAt, row.seq
				if createdAt.After(afterAt) || (createdAt.Equal(afterAt) && seq > afterSeq) {
					start = i
					break
				}
			}
		}
	} else {
		start = max(opts.Offset, 0)
	}
	if start >= len(rows) {
		return nil, "", nil
	}
	end := min(start+limit, len(rows))
	cursor := ""
	if end < len(rows) {
		last := rows[end-1]
		cursor = strconv.FormatInt(last.profile.CreatedAt.UnixNano(), 10) + ":" + strconv.FormatInt(last.seq, 10)
	}
	out := make([]jobs.EnrichmentProfile, 0, end-start)
	for _, row := range rows[start:end] {
		out = append(out, row.profile)
	}
	return out, cursor, nil
}
