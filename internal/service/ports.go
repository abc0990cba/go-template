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

// TicketsRepo persists ticket types, tickets, approval steps, comments, and
// timeline events. Listings are scope-aware: the caller narrows by creator
// (requester's own tickets), organization, or the approver's roles.
type TicketsRepo interface {
	CreateTicketType(ctx context.Context, typ model.TicketType) error
	GetTicketType(ctx context.Context, id string) (model.TicketType, error)
	GetTicketTypeByKey(ctx context.Context, orgID, key string) (model.TicketType, error)
	UpdateTicketType(ctx context.Context, typ model.TicketType) error
	DeleteTicketType(ctx context.Context, id string) error
	ListTicketTypes(ctx context.Context, orgID string) ([]model.TicketType, error)

	// CreateTicket stores the ticket and assigns its per-organization
	// sequence number, which is returned.
	CreateTicket(ctx context.Context, ticket model.Ticket) (int, error)
	GetTicket(ctx context.Context, id string) (model.Ticket, error)
	UpdateTicket(ctx context.Context, ticket model.Ticket) error
	DeleteTicket(ctx context.Context, id string) error
	ListTickets(ctx context.Context, opts repository.TicketOptions) ([]model.Ticket, string, error)
	CountTicketsByStatus(ctx context.Context, orgID, createdBy string) (map[string]int64, error)

	// ReplaceApprovals rewrites the ticket's approval steps (submit, reopen).
	ReplaceApprovals(ctx context.Context, ticketID string, steps []model.TicketApproval) error
	ListApprovals(ctx context.Context, ticketID string) ([]model.TicketApproval, error)
	UpdateApproval(ctx context.Context, approval model.TicketApproval) error
	// ListTicketsForApprover pages tickets whose current pending step
	// matches one of the given roles.
	ListTicketsForApprover(ctx context.Context, orgID string, roles []string, opts repository.ListOptions) ([]model.Ticket, string, error)
	CountTicketsForApprover(ctx context.Context, orgID string, roles []string) (int64, error)
	// ListDecidedApprovals returns the user's past decisions with their
	// tickets, newest first.
	ListDecidedApprovals(ctx context.Context, userID string, limit, offset int) ([]model.TicketDecision, error)

	AddTicketComment(ctx context.Context, comment model.TicketComment) error
	ListTicketComments(ctx context.Context, ticketID string, limit, offset int) ([]model.TicketComment, error)

	AddTicketEvent(ctx context.Context, event model.TicketEvent) error
	ListTicketEvents(ctx context.Context, ticketID string, limit, offset int) ([]model.TicketEvent, error)
}

// StreamsRepo applies enrichment events from the Kafka and NATS streams and
// manages the transactional outbox and the dead-letter queue. ApplyStreamEvent
// is exactly-once per (source, event_id) and atomic: dedupe check, per-source
// sequence guard, owned-fields merge, and the outbox write commit together.
type StreamsRepo interface {
	ApplyStreamEvent(ctx context.Context, event model.StreamEvent) (model.StreamApplyResult, error)

	// StreamSeqs reports the applied per-source sequence numbers of a user;
	// the demo trigger uses them to publish the next one.
	StreamSeqs(ctx context.Context, userID string) (profile, contact int64, err error)

	// Outbox: Claim leases a batch (SKIP LOCKED + stale-lease reclaim), the
	// relay publishes outside the lease, then marks processed or releases
	// with an attempts bump. FailPermanently parks exhausted events.
	ClaimOutboxBatch(ctx context.Context, limit, leaseSeconds, maxAttempts int) ([]model.OutboxEvent, error)
	MarkOutboxProcessed(ctx context.Context, ids []string) error
	ReleaseOutboxBatch(ctx context.Context, ids []string) error
	FailOutboxPermanently(ctx context.Context, ids []string) error
	CountOutboxPending(ctx context.Context) (int, error)
	ListOutbox(ctx context.Context, opts repository.ListOptions) ([]model.OutboxEvent, string, error)

	// Dead letters: poison or unprocessable messages parked for inspection.
	SaveDeadLetter(ctx context.Context, letter model.DeadLetter) error
	GetDeadLetter(ctx context.Context, id string) (model.DeadLetter, error)
	ListDeadLetters(ctx context.Context, opts repository.DeadLetterOptions) ([]model.DeadLetter, string, error)
	MarkDeadLetterReplayed(ctx context.Context, id string) error
	CountDeadLettersPending(ctx context.Context) (int, error)
}

// IdempotencyRepo stores API idempotency-key reservations and responses.
// Reserve inserts atomically: it reports already=true when the (user, key)
// pair exists, and the caller compares the stored record to decide between
// replay, rejection, and conflict.
type IdempotencyRepo interface {
	ReserveIdempotency(ctx context.Context, record model.IdempotencyRecord) (stored model.IdempotencyRecord, already bool, err error)
	CompleteIdempotency(ctx context.Context, key, userID string, status int, body []byte, contentType string) error
	PruneIdempotency(ctx context.Context) (int64, error)
}

// Compile-time proof that the in-memory fake stays interchangeable with any
// adapter, including the PostgreSQL one.
var (
	_ AuthRepo          = (*repository.MemoryStore)(nil)
	_ UsersRepo         = (*repository.MemoryStore)(nil)
	_ JobsRepo          = (*repository.MemoryStore)(nil)
	_ ReportsRepo       = (*repository.MemoryStore)(nil)
	_ AnalyticsRepo     = (*repository.MemoryStore)(nil)
	_ Pinger            = (*repository.MemoryStore)(nil)
	_ EventSink         = (*repository.MemoryStore)(nil)
	_ OrgsRepo          = (*repository.MemoryStore)(nil)
	_ RolesRepo         = (*repository.MemoryStore)(nil)
	_ AuditRepo         = (*repository.MemoryStore)(nil)
	_ NotificationsRepo = (*repository.MemoryStore)(nil)
	_ EmailTokensRepo   = (*repository.MemoryStore)(nil)
	_ APIKeysRepo       = (*repository.MemoryStore)(nil)
	_ TicketsRepo       = (*repository.MemoryStore)(nil)
	_ StreamsRepo       = (*repository.MemoryStore)(nil)
	_ IdempotencyRepo   = (*repository.MemoryStore)(nil)
)
