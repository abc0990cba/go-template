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

// Timeseries returns daily event counts for the last days days. It degrades
// to an empty series when no warehouse is configured or reachable — always a
// non-nil slice, so JSON clients see [] and chart libraries never choke.
func (s *AnalyticsService) Timeseries(ctx context.Context, days int) []model.EventDayPoint {
	if s.events == nil {
		return []model.EventDayPoint{}
	}
	points, err := s.events.EventTimeseries(ctx, days)
	if err != nil {
		s.logger.Warn("event timeseries unavailable", "err", err)
		return []model.EventDayPoint{}
	}
	if points == nil {
		points = []model.EventDayPoint{}
	}
	return points
}

// RecentEvents returns the newest events of the trail, newest first.
func (s *AnalyticsService) RecentEvents(ctx context.Context, limit int) []model.EventRecord {
	if s.events == nil {
		return []model.EventRecord{}
	}
	events, err := s.events.RecentEvents(ctx, limit)
	if err != nil {
		s.logger.Warn("recent events unavailable", "err", err)
		return []model.EventRecord{}
	}
	if events == nil {
		events = []model.EventRecord{}
	}
	return events
}


// validDimensions are the JSON properties the breakdown endpoint accepts;
// anything else is rejected to keep the query surface explicit.
var validDimensions = map[string]bool{"feature": true, "source": true, "country": true}

// Breakdown groups the window's events by one property dimension.
func (s *AnalyticsService) Breakdown(ctx context.Context, dimension string, days int) ([]model.NameCount, error) {
	if !validDimensions[dimension] {
		return nil, validationError("dimension must be feature, source, or country")
	}
	days = max(min(days, 90), 1)
	if s.events == nil {
		return []model.NameCount{}, nil
	}
	return s.events.EventBreakdown(ctx, dimension, days)
}

// DAU returns distinct active users per day over the window.
func (s *AnalyticsService) DAU(ctx context.Context, days int) ([]model.DayCount, error) {
	days = max(min(days, 90), 1)
	if s.events == nil {
		return []model.DayCount{}, nil
	}
	return s.events.DailyActiveUsers(ctx, days)
}
