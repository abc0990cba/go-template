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
	CreateSession(ctx context.Context, token, userID, impersonatedBy string, ttl time.Duration) error
	GetSession(ctx context.Context, token string, ttl time.Duration) (model.Session, error)
	DeleteSession(ctx context.Context, token string) error
	DeleteSessionsForUser(ctx context.Context, userID string) error
	LinkIdentity(ctx context.Context, identity model.AuthIdentity) error
	GetIdentity(ctx context.Context, provider, subject string) (model.AuthIdentity, error)
	UnlinkIdentity(ctx context.Context, userID, provider string) error
	ListIdentities(ctx context.Context, userID string) ([]model.AuthIdentity, error)
	ListSessions(ctx context.Context, userID string) ([]model.SessionInfo, error)
	SaveOAuthState(ctx context.Context, state model.OAuthState) error
	ConsumeOAuthState(ctx context.Context, state string, now time.Time) (model.OAuthState, error)
	// SetPasswordCredential upserts the password credential; "" removes it.
	SetPasswordCredential(ctx context.Context, userID, passwordHash string) error
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

// JobsRepo persists job records, enrichment results, and their paginated
// listings.
type JobsRepo interface {
	SaveJob(ctx context.Context, job model.Job) error
	GetJob(ctx context.Context, id string) (model.Job, error)
	ListJobs(ctx context.Context, opts repository.ListOptions) ([]model.Job, string, error)
	AddEnrichment(ctx context.Context, profile model.EnrichmentProfile) error
	ListEnrichments(ctx context.Context, opts repository.ListOptions) ([]model.EnrichmentProfile, string, error)
}

// ReportsRepo persists reports and exposes the analytics summary they
// snapshot.
type ReportsRepo interface {
	SaveReport(ctx context.Context, report model.Report) error
	GetReport(ctx context.Context, id string) (model.Report, error)
	ListReports(ctx context.Context, opts repository.ListOptions) ([]model.Report, string, error)
	Analytics(ctx context.Context) model.AnalyticsSummary
}

// AnalyticsRepo reads the dashboard summary.
type AnalyticsRepo interface {
	Analytics(ctx context.Context) model.AnalyticsSummary
}

// EventSink receives named analytics events tied to the acting user;
// implementations may buffer, persist, or drop them.
type EventSink interface {
	AddEvent(ctx context.Context, event, userID string)
}

// EventStats reads aggregates and slices of the event trail from the
// analytics warehouse (ClickHouse, or the in-memory trail in dev mode).
type EventStats interface {
	EventCounts(ctx context.Context) (map[string]int64, error)
	EventTimeseries(ctx context.Context, days int) ([]model.EventDayPoint, error)
	RecentEvents(ctx context.Context, limit int) ([]model.EventRecord, error)
	// EventBreakdown groups the window's events by one JSON property
	// dimension (feature, source, country).
	EventBreakdown(ctx context.Context, dimension string, days int) ([]model.NameCount, error)
	// DailyActiveUsers counts distinct event-attributed users per day.
	DailyActiveUsers(ctx context.Context, days int) ([]model.DayCount, error)
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

// OrgsRepo manages organizations (Casbin domains) and their memberships.
type OrgsRepo interface {
	CreateOrg(ctx context.Context, org model.Organization) error
	GetOrg(ctx context.Context, id string) (model.Organization, error)
	UpdateOrg(ctx context.Context, org model.Organization) error
	DeleteOrg(ctx context.Context, id string) error
	ListOrgs(ctx context.Context, opts repository.ListOptions) ([]model.Organization, string, error)
	ListOrgsForUser(ctx context.Context, userID string) ([]model.Organization, error)
	AddOrgMember(ctx context.Context, member model.OrgMember) error
	RemoveOrgMember(ctx context.Context, orgID, userID string) error
	IsOrgMember(ctx context.Context, orgID, userID string) (bool, error)
	ListOrgMembers(ctx context.Context, orgID string) ([]model.OrgMember, error)
}

// RolesRepo persists role metadata — the display half of RBAC. The
// permissions themselves are enforced by the policy engine.
type RolesRepo interface {
	CreateRole(ctx context.Context, role model.Role) error
	GetRole(ctx context.Context, key string) (model.Role, error)
	UpdateRole(ctx context.Context, role model.Role) error
	DeleteRole(ctx context.Context, key string) error
	ListRoles(ctx context.Context) ([]model.Role, error)
}

// AuditRepo appends immutable audit records and lists them for the admin
// console. Records are never updated or deleted except by retention.
type AuditRepo interface {
	RecordAudit(ctx context.Context, entry model.AuditEntry) error
	ListAudit(ctx context.Context, opts repository.AuditOptions) ([]model.AuditEntry, error)
}

// NotificationsRepo persists per-user in-app notifications.
type NotificationsRepo interface {
	CreateNotification(ctx context.Context, notification model.Notification) error
	ListNotifications(ctx context.Context, userID string, limit, offset int) ([]model.Notification, error)
	CountUnreadNotifications(ctx context.Context, userID string) (int64, error)
	MarkNotificationRead(ctx context.Context, id, userID string) error
	MarkAllNotificationsRead(ctx context.Context, userID string) error
}

// EmailTokensRepo persists single-use hashed tokens for password resets
// and email verification.
type EmailTokensRepo interface {
	CreateEmailToken(ctx context.Context, token model.EmailToken, tokenHash string) error
	GetEmailTokenByHash(ctx context.Context, tokenHash string) (model.EmailToken, error)
	MarkEmailTokenUsed(ctx context.Context, id string) error
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
	_ EventSink     = (*repository.MemoryStore)(nil)
	_ OrgsRepo      = (*repository.MemoryStore)(nil)
	_ RolesRepo     = (*repository.MemoryStore)(nil)
	_ AuditRepo     = (*repository.MemoryStore)(nil)
	_ NotificationsRepo = (*repository.MemoryStore)(nil)
	_ EmailTokensRepo = (*repository.MemoryStore)(nil)
	_ APIKeysRepo = (*repository.MemoryStore)(nil)
)
