package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/postgres/db"
)

// SaveJob upserts the full job record.
func (s *Store) SaveJob(ctx context.Context, job jobs.Job) error {
	return mapErr(s.q.UpsertJob(ctx, db.UpsertJobParams{
		ID: job.ID, Type: job.Type, Status: job.Status, Column4: job.Payload,
		Error: job.Error, OrgID: job.OrgID, CreatedBy: job.CreatedBy,
		CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
	}))
}

// GetJob returns a job by id.
func (s *Store) GetJob(ctx context.Context, id string) (jobs.Job, error) {
	row, err := s.q.GetJob(ctx, id)
	if err != nil {
		return jobs.Job{}, mapErr(err)
	}
	return jobs.Job{
		ID: row.ID, Type: row.Type, Status: row.Status, Payload: row.Payload,
		Error: row.Error, OrgID: row.OrgID, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

// ListJobs pages the operations job list. The shared pagination strategy
// applies to the keyset/offset query pair: limit is fetched with one extra
// row to detect a next page, and the returned cursor is the last emitted
// row's key.
func (s *Store) ListJobs(ctx context.Context, opts kernel.ListOptions) ([]jobs.Job, string, error) {
	const def = 25
	limit := int32(def)
	if opts.Limit > 0 && opts.Limit <= 100 {
		limit = int32(opts.Limit)
	}
	if opts.Mode == "cursor" || opts.Mode == "keyset" {
		params := db.ListJobsKeysetParams{Query: opts.Query, Status: opts.Status, Typ: opts.Type, After: opts.After, Lim: limit + 1}
		if opts.After == "" { // first keyset page: no continuation predicate
			rows, err := s.q.ListJobsOffset(ctx, db.ListJobsOffsetParams{Query: opts.Query, Status: opts.Status, Typ: opts.Type, Off: 0, Lim: limit + 1})
			if err != nil {
				return nil, "", mapErr(err)
			}
			jobs, next := jobsFromRows(offsetToJobs(rows), limit)
			return jobs, next, nil
		}
		rows, err := s.q.ListJobsKeyset(ctx, params)
		if err != nil {
			return nil, "", mapErr(err)
		}
		jobs, next := jobsFromRows(keysetToJobs(rows), limit)
		return jobs, next, nil
	}
	off := int32(0)
	if opts.Offset > 0 {
		off = int32(min(opts.Offset, 1<<31-1)) // #nosec G115 -- clamped to int32 range
	}
	rows, err := s.q.ListJobsOffset(ctx, db.ListJobsOffsetParams{Query: opts.Query, Status: opts.Status, Typ: opts.Type, Off: off, Lim: limit + 1})
	if err != nil {
		return nil, "", mapErr(err)
	}
	jobs, next := jobsFromRows(offsetToJobs(rows), limit)
	return jobs, next, nil
}

func keysetToJobs(rows []db.ListJobsKeysetRow) []jobs.Job {
	out := make([]jobs.Job, 0, len(rows))
	for _, row := range rows {
		out = append(out, jobs.Job{
			ID: row.ID, Type: row.Type, Status: row.Status, Payload: row.Payload,
			Error: row.Error, OrgID: row.OrgID, CreatedBy: row.CreatedBy,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	return out
}

func offsetToJobs(rows []db.ListJobsOffsetRow) []jobs.Job {
	out := make([]jobs.Job, 0, len(rows))
	for _, row := range rows {
		out = append(out, jobs.Job{
			ID: row.ID, Type: row.Type, Status: row.Status, Payload: row.Payload,
			Error: row.Error, OrgID: row.OrgID, CreatedBy: row.CreatedBy,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	return out
}

// jobsFromRows trims the limit+1th row and derives the next cursor.
func jobsFromRows(listed []jobs.Job, limit int32) ([]jobs.Job, string) {
	next := ""
	if len(listed) > int(limit) {
		listed = listed[:limit]
		next = listed[limit-1].ID
	}
	return listed, next
}

// ListEnrichments pages the append-only enrichment trail. Its keyset cursor
// carries both created_at and the seq column: "<unix nano>:<seq>".
func (s *Store) ListEnrichments(ctx context.Context, opts kernel.ListOptions) ([]jobs.EnrichmentProfile, string, error) {
	const def = 25
	limit := int32(def)
	if opts.Limit > 0 && opts.Limit <= 100 {
		limit = int32(opts.Limit)
	}
	var (
		rows []db.UserEnrichmentProfile
		err  error
	)
	if opts.Mode == "cursor" || opts.Mode == "keyset" {
		if opts.After == "" {
			rows, err = s.q.ListEnrichmentsOffset(ctx, db.ListEnrichmentsOffsetParams{Query: opts.Query, UserID: opts.UserID, Off: 0, Lim: limit + 1})
		} else {
			afterAt, afterSeq, ok := kernel.SplitEnrichmentCursor(opts.After)
			if !ok {
				return nil, "", fmt.Errorf("%w: malformed enrichment cursor", kernel.ErrNotFound)
			}
			rows, err = s.q.ListEnrichmentsKeyset(ctx, db.ListEnrichmentsKeysetParams{
				Query: opts.Query, UserID: opts.UserID, AfterAt: afterAt,
				AfterSeq: pgtype.Int8{Int64: afterSeq, Valid: true}, Lim: limit + 1,
			})
		}
	} else {
		off := int32(0)
		if opts.Offset > 0 {
			off = int32(min(opts.Offset, 1<<31-1)) // #nosec G115 -- clamped to int32 range
		}
		rows, err = s.q.ListEnrichmentsOffset(ctx, db.ListEnrichmentsOffsetParams{Query: opts.Query, UserID: opts.UserID, Off: off, Lim: limit + 1})
	}
	if err != nil {
		return nil, "", mapErr(err)
	}
	next := ""
	if len(rows) > int(limit) {
		rows = rows[:limit]
		last := rows[limit-1]
		next = strconv.FormatInt(last.CreatedAt.UnixNano(), 10) + ":" + strconv.FormatInt(last.Seq.Int64, 10)
	}
	profiles := make([]jobs.EnrichmentProfile, 0, len(rows))
	for _, row := range rows {
		profiles = append(profiles, jobs.EnrichmentProfile{
			UserID: row.UserID, Source: row.Source, Payload: row.Payload, CreatedAt: row.CreatedAt,
		})
	}
	return profiles, next, nil
}

// AddEnrichment appends one enrichment profile.
func (s *Store) AddEnrichment(ctx context.Context, profile jobs.EnrichmentProfile) error {
	return mapErr(s.q.AddEnrichment(ctx, db.AddEnrichmentParams{
		UserID: profile.UserID, Source: profile.Source, Column3: profile.Payload, CreatedAt: profile.CreatedAt,
	}))
}

// SaveReport upserts the report.
func (s *Store) SaveReport(ctx context.Context, report analytics.Report) error {
	return mapErr(s.q.UpsertReport(ctx, db.UpsertReportParams{
		ID: report.ID, Status: report.Status, Result: report.Result, Error: report.Error,
		OrgID: report.OrgID, CreatedAt: report.CreatedAt, UpdatedAt: report.UpdatedAt,
	}))
}

// GetReport returns a report by id.
func (s *Store) GetReport(ctx context.Context, id string) (analytics.Report, error) {
	row, err := s.q.GetReport(ctx, id)
	if err != nil {
		return analytics.Report{}, mapErr(err)
	}
	return analytics.Report{
		ID: row.ID, Status: row.Status, Result: row.Result, Error: row.Error,
		OrgID: row.OrgID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

// ListReports pages reports newest first; cursor/keyset continue after
// After, and the returned cursor is empty on the last page.
func (s *Store) ListReports(ctx context.Context, opts kernel.ListOptions) ([]analytics.Report, string, error) {
	limit := int32(25)
	if opts.Limit > 0 && opts.Limit <= 100 {
		limit = int32(opts.Limit)
	}
	listFrom := func(rows []db.ListReportsOffsetRow) ([]analytics.Report, string, error) {
		reports := make([]analytics.Report, 0, len(rows))
		for _, row := range rows {
			reports = append(reports, analytics.Report{
				ID: row.ID, Status: row.Status, Result: row.Result, Error: row.Error,
				OrgID: row.OrgID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			})
		}
		next := ""
		if len(reports) > int(limit) {
			reports = reports[:limit]
			next = reports[limit-1].ID
		}
		return reports, next, nil
	}
	if opts.Mode == "cursor" || opts.Mode == "keyset" {
		if opts.After == "" {
			rows, err := s.q.ListReportsOffset(ctx, db.ListReportsOffsetParams{Query: opts.Query, Off: 0, Lim: limit + 1})
			if err != nil {
				return nil, "", mapErr(err)
			}
			return listFrom(rows)
		}
		rows, err := s.q.ListReportsKeyset(ctx, db.ListReportsKeysetParams{Query: opts.Query, After: opts.After, Lim: limit + 1})
		if err != nil {
			return nil, "", mapErr(err)
		}
		reports := make([]analytics.Report, 0, len(rows))
		for _, row := range rows {
			reports = append(reports, analytics.Report{
				ID: row.ID, Status: row.Status, Result: row.Result, Error: row.Error,
				OrgID: row.OrgID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			})
		}
		next := ""
		if len(reports) > int(limit) {
			reports = reports[:limit]
			next = reports[limit-1].ID
		}
		return reports, next, nil
	}
	off := int32(0)
	if opts.Offset > 0 {
		off = int32(min(opts.Offset, 1<<31-1)) // #nosec G115 -- clamped to int32 range
	}
	rows, err := s.q.ListReportsOffset(ctx, db.ListReportsOffsetParams{Query: opts.Query, Off: off, Lim: limit + 1})
	if err != nil {
		return nil, "", mapErr(err)
	}
	return listFrom(rows)
}

// AddEvent is a no-op: events persist in ClickHouse, and operational
// counters do not depend on it.
func (s *Store) AddEvent(_ context.Context, _ string, _ string) {}

// ResetDemoData removes everything the demo seed created: demo users
// (credentials, identities, and sessions cascade), jobs, reports, and
// enrichment profiles. It backs bootstrap's optional reset extension.
func (s *Store) ResetDemoData(ctx context.Context) error {
	return s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.DeleteDemoJobs(ctx); err != nil {
			return mapErr(err)
		}
		if _, err := q.DeleteDemoReports(ctx); err != nil {
			return mapErr(err)
		}
		if _, err := q.DeleteDemoEnrichments(ctx); err != nil {
			return mapErr(err)
		}
		if _, err := q.DeleteDemoOrgs(ctx); err != nil {
			return mapErr(err)
		}
		if _, err := q.DeleteDemoNotifications(ctx); err != nil {
			return mapErr(err)
		}
		if _, err := q.DeleteDemoAPIKeys(ctx); err != nil {
			return mapErr(err)
		}
		if _, err := q.DeleteDemoEmailTokens(ctx); err != nil {
			return mapErr(err)
		}
		if _, err := q.DeleteDemoWebhookEndpoints(ctx); err != nil {
			return mapErr(err)
		}
		if _, err := q.DeleteDemoTickets(ctx); err != nil {
			return mapErr(err)
		}
		if _, err := q.DeleteTicketsReferencingDemoTypes(ctx); err != nil {
			return mapErr(err)
		}
		if _, err := q.DeleteDemoTicketTypes(ctx); err != nil {
			return mapErr(err)
		}
		if _, err := q.DeleteDemoAudit(ctx); err != nil {
			return mapErr(err)
		}
		if _, err := q.DeleteDemoProcessedEvents(ctx); err != nil {
			return mapErr(err)
		}
		if _, err := q.DeleteDemoOutbox(ctx); err != nil {
			return mapErr(err)
		}
		_, err := q.DeleteDemoUsers(ctx)
		return mapErr(err)
	})
}

// Analytics computes dashboard counters with filtered subqueries.
func (s *Store) Analytics(ctx context.Context) analytics.Summary {
	row, err := s.q.Analytics(ctx)
	if err != nil {
		return analytics.Summary{}
	}
	return analytics.Summary{
		TotalUsers:          int(row.TotalUsers),
		PasswordIdentities:  int(row.PasswordIdentities),
		GoogleIdentities:    int(row.GoogleIdentities),
		GitHubIdentities:    int(row.GithubIdentities),
		FacebookIdentities:  int(row.FacebookIdentities),
		TwitterIdentities:   int(row.TwitterIdentities),
		EnrichmentSucceeded: int(row.EnrichmentSucceeded),
		JobsQueued:          int(row.JobsQueued),
		JobsDone:            int(row.JobsDone),
		JobsFailed:          int(row.JobsFailed),
	}
}
