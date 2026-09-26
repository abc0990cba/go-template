package service

import (
	"context"
	"fmt"
	"time"

	"github.com/your-org/go-template/internal/model"
)

// ReportService produces snapshot reports from the analytics summary.
type ReportService struct {
	store ReportsRepo
}

// Generate computes the summary and stores it as a finished report.
func (s *ReportService) Generate(ctx context.Context) (model.Report, error) {
	summary := s.store.Analytics(ctx)
	now := time.Now()
	report := model.Report{ID: newID(), Status: "done", Result: formatSummary(summary), CreatedAt: now, UpdatedAt: now}
	return report, s.store.SaveReport(ctx, report)
}

// List returns all stored reports.
func (s *ReportService) List(ctx context.Context) ([]model.Report, error) {
	return s.store.ListReports(ctx)
}

// formatSummary renders the canonical one-line report body shared by the
// synchronous API and the background job.
func formatSummary(summary model.AnalyticsSummary) string {
	return fmt.Sprintf("users=%d password=%d google=%d github=%d jobs_done=%d",
		summary.TotalUsers, summary.PasswordIdentities, summary.GoogleIdentities, summary.GitHubIdentities, summary.JobsDone)
}
