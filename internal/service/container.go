package service

import (
	"log/slog"
	"net/http"

	"github.com/your-org/go-template/internal/config"
)

// Container wires every service against the store ports; handlers depend on
// this aggregate rather than on individual services.
type Container struct {
	Auth      *AuthService
	Users     *UserService
	Jobs      *JobService
	Reports   *ReportService
	Analytics *AnalyticsService
}

// NewContainer builds the service graph. One store usually satisfies every
// port, so callers pass the same adapter five times; a memory fake and a
// PostgreSQL adapter are interchangeable. queue may be nil for the
// single-process development mode.
func NewContainer(auth AuthRepo, users UsersRepo, jobs JobsRepo, reports ReportsRepo, analytics AnalyticsRepo, queue JobQueue, observer JobObserver, logger *slog.Logger, cfg config.Config) *Container {
	return &Container{
		Auth:  &AuthService{store: auth},
		Users: &UserService{store: users},
		Jobs: &JobService{
			store:          jobs,
			reports:        reports,
			queue:          queue,
			observer:       observer,
			logger:         logger,
			client:         &http.Client{Timeout: cfg.ExternalTimeout},
			maxConcurrency: cfg.MaxConcurrency,
		},
		Reports:   &ReportService{store: reports},
		Analytics: &AnalyticsService{store: analytics},
	}
}
