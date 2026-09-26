package service

import (
	"context"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
)

// AnalyticsService reads the dashboard summary from the store.
type AnalyticsService struct {
	store *repository.MemoryStore
}

// Summary computes aggregate counters for the dashboard and reports.
func (s *AnalyticsService) Summary(ctx context.Context) model.AnalyticsSummary {
	return s.store.Analytics(ctx)
}
