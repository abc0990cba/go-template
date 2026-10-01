package analytics

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/your-org/go-template/internal/kernel"
)

// ReportStore persists reports and exposes the analytics summary they
// snapshot.
type ReportStore interface {
	SaveReport(ctx context.Context, report Report) error
	GetReport(ctx context.Context, id string) (Report, error)
	ListReports(ctx context.Context, opts kernel.ListOptions) ([]Report, string, error)
	Analytics(ctx context.Context) AnalyticsSummary
}

// Sink receives named analytics events tied to the acting user;
// implementations may buffer, persist, or drop them.
type Sink interface {
	AddEvent(ctx context.Context, event, userID string)
}

// Stats reads aggregates and slices of the event trail from the
// analytics warehouse (ClickHouse, or the in-memory trail in dev mode).
type Stats interface {
	EventCounts(ctx context.Context) (map[string]int64, error)
	EventTimeseries(ctx context.Context, days int) ([]EventDayPoint, error)
	RecentEvents(ctx context.Context, limit int) ([]EventRecord, error)
	// EventBreakdown groups the window's events by one JSON property
	// dimension (feature, source, country).
	EventBreakdown(ctx context.Context, dimension string, days int) ([]NameCount, error)
	// DailyActiveUsers counts distinct event-attributed users per day.
	DailyActiveUsers(ctx context.Context, days int) ([]DayCount, error)
}

// ReportService produces snapshot reports from the analytics summary.
type ReportService struct {
	store ReportStore
}

// NewReportService builds the report service on the given store.
func NewReportService(store ReportStore) *ReportService {
	return &ReportService{store: store}
}

// Generate computes the summary and stores it as a finished report.
func (s *ReportService) Generate(ctx context.Context) (Report, error) {
	summary := s.store.Analytics(ctx)
	now := time.Now()
	report := Report{ID: kernel.NewID(), Status: "done", Result: FormatSummary(summary), CreatedAt: now, UpdatedAt: now}
	return report, s.store.SaveReport(ctx, report)
}

// List returns one page of reports plus the next-page cursor, newest first.
func (s *ReportService) List(ctx context.Context, opts kernel.ListOptions) ([]Report, string, error) {
	return s.store.ListReports(ctx, opts)
}

// Get returns a report by id.
func (s *ReportService) Get(ctx context.Context, id string) (Report, error) {
	return s.store.GetReport(ctx, id)
}

// FormatSummary renders the canonical one-line report body shared by the
// synchronous API and the background job.
func FormatSummary(summary AnalyticsSummary) string {
	return fmt.Sprintf("users=%d password=%d google=%d github=%d jobs_done=%d",
		summary.TotalUsers, summary.PasswordIdentities, summary.GoogleIdentities, summary.GitHubIdentities, summary.JobsDone)
}

// AnalyticsService reads the dashboard summary from the store and, when an
// event warehouse is configured, merges its event counters.
type AnalyticsService struct {
	store  ReportStore
	events Stats
	logger *slog.Logger
}

// NewAnalyticsService builds the analytics service. The summary comes from
// the report store's Analytics query; the warehouse stats may be nil.
func NewAnalyticsService(store ReportStore, events Stats, logger *slog.Logger) *AnalyticsService {
	return &AnalyticsService{store: store, events: events, logger: logger}
}

// Summary computes aggregate counters for the dashboard and reports. A
// warehouse outage degrades to the operational counters, never an error.
func (s *AnalyticsService) Summary(ctx context.Context) AnalyticsSummary {
	summary := s.store.Analytics(ctx)
	if s.events == nil {
		return summary
	}
	counts, err := s.events.EventCounts(ctx)
	if err != nil {
		s.logger.Warn("event stats unavailable; serving operational counters", "err", err)
		return summary
	}
	summary.EventsRegistered = int(counts["user_registered"])
	summary.EventsLoggedIn = int(counts["user_logged_in"])
	summary.EventsOAuthLinked = int(counts["oauth_linked"])
	summary.EventsJobsQueued = int(counts["job_queued"])
	summary.EventsJobsDone = int(counts["job_done"])
	summary.EventsJobsFailed = int(counts["job_failed"])
	return summary
}

// Timeseries returns daily event counts for the last days days. It degrades
// to an empty series when no warehouse is configured or reachable — always a
// non-nil slice, so JSON clients see [] and chart libraries never choke.
func (s *AnalyticsService) Timeseries(ctx context.Context, days int) []EventDayPoint {
	if s.events == nil {
		return []EventDayPoint{}
	}
	points, err := s.events.EventTimeseries(ctx, days)
	if err != nil {
		s.logger.Warn("event timeseries unavailable", "err", err)
		return []EventDayPoint{}
	}
	if points == nil {
		points = []EventDayPoint{}
	}
	return points
}

// RecentEvents returns the newest events of the trail, newest first.
func (s *AnalyticsService) RecentEvents(ctx context.Context, limit int) []EventRecord {
	if s.events == nil {
		return []EventRecord{}
	}
	events, err := s.events.RecentEvents(ctx, limit)
	if err != nil {
		s.logger.Warn("recent events unavailable", "err", err)
		return []EventRecord{}
	}
	if events == nil {
		events = []EventRecord{}
	}
	return events
}

// validDimensions are the JSON properties the breakdown endpoint accepts;
// anything else is rejected to keep the query surface explicit.
var validDimensions = map[string]bool{"feature": true, "source": true, "country": true}

// Breakdown groups the window's events by one property dimension.
func (s *AnalyticsService) Breakdown(ctx context.Context, dimension string, days int) ([]NameCount, error) {
	if !validDimensions[dimension] {
		return nil, kernel.Invalid("dimension must be feature, source, or country")
	}
	days = max(min(days, 90), 1)
	if s.events == nil {
		return []NameCount{}, nil
	}
	return s.events.EventBreakdown(ctx, dimension, days)
}

// DAU returns distinct active users per day over the window.
func (s *AnalyticsService) DAU(ctx context.Context, days int) ([]DayCount, error) {
	days = max(min(days, 90), 1)
	if s.events == nil {
		return []DayCount{}, nil
	}
	return s.events.DailyActiveUsers(ctx, days)
}
