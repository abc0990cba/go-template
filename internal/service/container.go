package service

import (
	"log/slog"
	"net/http"

	"github.com/your-org/go-template/internal/config"
	"github.com/your-org/go-template/internal/repository"
)

// Container wires every service against one store; handlers depend on this
// aggregate rather than on individual services.
type Container struct {
	Auth      *AuthService
	Users     *UserService
	Jobs      *JobService
	Reports   *ReportService
	Analytics *AnalyticsService
}

// NewContainer builds the service graph on top of a store.
func NewContainer(store *repository.MemoryStore, logger *slog.Logger, cfg config.Config) *Container {
	jobs := &JobService{
		store:          store,
		logger:         logger,
		client:         &http.Client{Timeout: cfg.ExternalTimeout},
		maxConcurrency: cfg.MaxConcurrency,
	}
	return &Container{
		Auth:      &AuthService{store: store},
		Users:     &UserService{store: store},
		Jobs:      jobs,
		Reports:   &ReportService{store: store},
		Analytics: &AnalyticsService{store: store},
	}
}
