package service

import (
	"context"
	"log/slog"

	"github.com/your-org/go-template/internal/model"
)

// AnalyticsService reads the dashboard summary from the store and, when an
// event warehouse is configured, merges its event counters.
type AnalyticsService struct {
	store  AnalyticsRepo
	events EventStats
	logger *slog.Logger
}

// Summary computes aggregate counters for the dashboard and reports. A
// warehouse outage degrades to the operational counters, never an error.
func (s *AnalyticsService) Summary(ctx context.Context) model.AnalyticsSummary {
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
