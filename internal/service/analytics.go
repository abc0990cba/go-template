package service

import (
	"context"

	"github.com/your-org/go-template/internal/model"
)

// AnalyticsService reads the dashboard summary from the store.
type AnalyticsService struct {
	store AnalyticsRepo
}

// Summary computes aggregate counters for the dashboard and reports.
func (s *AnalyticsService) Summary(ctx context.Context) model.AnalyticsSummary {
	return s.store.Analytics(ctx)
}
