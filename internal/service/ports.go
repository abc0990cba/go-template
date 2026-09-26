package service

import (
	"context"
	"time"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
)

// The service layer depends on these ports, not on a concrete store. The
// in-memory fake and the PostgreSQL adapter both implement them; contract
// tests keep the two interchangeable.

// AuthRepo persists users, credentials, identities, sessions, and OAuth
// state for the auth flows.
type AuthRepo interface {
	CreateUser(ctx context.Context, user model.User, passwordHash string) error
	GetUser(ctx context.Context, id string) (model.User, error)
	GetUserByEmail(ctx context.Context, email string) (model.User, string, error)
	CreateSession(ctx context.Context, token, userID string, ttl time.Duration) error
	GetSession(ctx context.Context, token string, ttl time.Duration) (string, error)
	DeleteSession(ctx context.Context, token string) error
	DeleteSessionsForUser(ctx context.Context, userID string) error
	LinkIdentity(ctx context.Context, identity model.AuthIdentity) error
	GetIdentity(ctx context.Context, provider, subject string) (model.AuthIdentity, error)
	UnlinkIdentity(ctx context.Context, userID, provider string) error
	ListIdentities(ctx context.Context, userID string) ([]model.AuthIdentity, error)
	SaveOAuthState(ctx context.Context, state model.OAuthState) error
	ConsumeOAuthState(ctx context.Context, state string, now time.Time) (model.OAuthState, error)
	AddEvent(ctx context.Context, event string)
}

// UsersRepo persists user records for management flows.
type UsersRepo interface {
	CreateUser(ctx context.Context, user model.User, passwordHash string) error
	UpdateUser(ctx context.Context, user model.User) error
	DeleteUser(ctx context.Context, id string) error
	GetUser(ctx context.Context, id string) (model.User, error)
	ListUsers(ctx context.Context, opts repository.ListUsersOptions) ([]model.User, string, error)
	CountUsers(ctx context.Context) int
}

// JobsRepo persists job records and enrichment results.
type JobsRepo interface {
	SaveJob(ctx context.Context, job model.Job) error
	GetJob(ctx context.Context, id string) (model.Job, error)
	ListJobs(ctx context.Context) ([]model.Job, error)
	AddEnrichment(ctx context.Context, profile model.EnrichmentProfile) error
	AddEvent(ctx context.Context, event string)
}

// ReportsRepo persists reports and exposes the analytics summary they
// snapshot.
type ReportsRepo interface {
	SaveReport(ctx context.Context, report model.Report) error
	ListReports(ctx context.Context) ([]model.Report, error)
	Analytics(ctx context.Context) model.AnalyticsSummary
}

// AnalyticsRepo reads the dashboard summary and records named events.
type AnalyticsRepo interface {
	Analytics(ctx context.Context) model.AnalyticsSummary
	AddEvent(ctx context.Context, event string)
}

// Pinger reports whether the backing dependencies are ready; health
// endpoints surface it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// JobObserver receives job lifecycle notifications; the metrics stack
// implements it without coupling the domain to Prometheus.
type JobObserver interface {
	JobQueued(ctx context.Context)
	JobFinished(ctx context.Context, typ, status string, duration time.Duration)
}

// Compile-time proof that the in-memory fake stays interchangeable with any
// adapter, including the PostgreSQL one.
var (
	_ AuthRepo      = (*repository.MemoryStore)(nil)
	_ UsersRepo     = (*repository.MemoryStore)(nil)
	_ JobsRepo      = (*repository.MemoryStore)(nil)
	_ ReportsRepo   = (*repository.MemoryStore)(nil)
	_ AnalyticsRepo = (*repository.MemoryStore)(nil)
	_ Pinger        = (*repository.MemoryStore)(nil)
)
