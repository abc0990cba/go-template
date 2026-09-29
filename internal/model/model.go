// Package model defines the domain types shared across layers. JSON tags are
// part of the API contract: response fields are snake_case.
package model

import "time"

// User is an account in the system. ExternalRef is the correlation id the
// external systems use for this account; the enrichment streams (Kafka
// profile, NATS contact) merge into EnrichProfile / EnrichContact, each
// owned by exactly one source.
type User struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	Name          string     `json:"name"`
	ExternalRef   string     `json:"external_ref,omitempty"`
	EnrichProfile string     `json:"enrich_profile,omitempty"`
	EnrichContact string     `json:"enrich_contact,omitempty"`
	EnrichedAt    *time.Time `json:"enriched_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// AuthIdentity links one external identity (password, google, github) to a
// user; (provider, provider_subject) is globally unique.
type AuthIdentity struct {
	UserID          string    `json:"user_id"`
	Provider        string    `json:"provider"`
	ProviderSubject string    `json:"provider_subject"`
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	AvatarURL       string    `json:"avatar_url"`
	LinkedAt        time.Time `json:"linked_at"`
	LastLoginAt     time.Time `json:"last_login_at"`
}

// OAuthState is a single-use CSRF value for the OAuth redirect round trip.
type OAuthState struct {
	State         string    `json:"state"`
	Provider      string    `json:"provider"`
	UserID        string    `json:"user_id"`
	RedirectAfter string    `json:"redirect_after"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// OAuthProfile is the normalized profile returned by a provider after the
// code exchange.
type OAuthProfile struct {
	Provider        string `json:"provider"`
	ProviderSubject string `json:"provider_subject"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	AvatarURL       string `json:"avatar_url"`
}

// EnrichmentProfile is one fetched external profile record.
type EnrichmentProfile struct {
	UserID    string    `json:"user_id"`
	Source    string    `json:"source"`
	Payload   string    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}

// Session is the resolution of an opaque session token: the user it
// belongs to and, for impersonation sessions, the admin behind it.
type Session struct {
	UserID         string
	ImpersonatedBy string
}

// SessionInfo describes one active session of a user. ID is a short prefix
// of the opaque token — enough to tell sessions apart, useless to resume one.
type SessionInfo struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// EventRecord is a single analytics event from the trail, as delivered by
// the SSE stream and the recent-events endpoint. Properties carries a JSON
// object of dimensions (feature, source, country) for breakdowns.
type EventRecord struct {
	Name       string    `json:"name"`
	OccurredAt time.Time `json:"occurred_at"`
	UserID     string    `json:"user_id,omitempty"`
	Properties string    `json:"properties,omitempty"`
}

// NameCount is one labeled aggregate: a breakdown row or a counter.
type NameCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// DayCount is one day of a daily aggregate such as DAU.
type DayCount struct {
	Day   string `json:"day"`
	Count int64  `json:"count"`
}

// EventDayPoint aggregates one calendar day of the event trail by name;
// charts render it as a stacked series.
type EventDayPoint struct {
	Day    string           `json:"day"` // YYYY-MM-DD
	Events map[string]int64 `json:"events"`
	Total  int64            `json:"total"`
}

// OAuthProviderInfo tells the UI whether an identity provider is configured,
// so link buttons render disabled instead of failing at click time.
type OAuthProviderInfo struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}

// Job is a background task record with its lifecycle status. OrgID and
// CreatedBy attribute the job to the organization context and user that
// queued it (empty for platform/anonymous runs).
type Job struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Payload   string    `json:"payload"`
	Error     string    `json:"error"`
	OrgID     string    `json:"org_id,omitempty"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Report is a stored analytics snapshot.
type Report struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Result    string    `json:"result"`
	Error     string    `json:"error"`
	OrgID     string    `json:"org_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Organization is a tenant: a Casbin domain grouping users and scoping
// policies. The reserved domain "*" means the whole platform.
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrgMember is one membership with the user snapshot the members list
// renders. Role assignments live in the policy engine, not here.
type OrgMember struct {
	OrgID    string    `json:"org_id"`
	UserID   string    `json:"user_id"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	JoinedAt time.Time `json:"joined_at"`
}

// Role is the human-readable metadata of a Casbin role key. System roles
// cannot be renamed away or deleted from the admin UI.
type Role struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsSystem    bool      `json:"is_system"`
	CreatedAt   time.Time `json:"created_at"`
}

// PolicyRule is one stored policy (ptype "p": subject, domain, object,
// action) or grouping rule (ptype "g": user, role, domain) as shown by the
// raw policy editor.
type PolicyRule struct {
	PType   string `json:"ptype"`
	Subject string `json:"subject"`
	Domain  string `json:"domain"`
	Object  string `json:"object,omitempty"`
	Action  string `json:"action,omitempty"`
}

// AuditEntry is one immutable audit record: an actor performed action on a
// resource, with the request context and an optional before/after diff.
type AuditEntry struct {
	ID           string    `json:"id"`
	ActorID      string    `json:"actor_id"`
	ActorEmail   string    `json:"actor_email"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id,omitempty"`
	Domain       string    `json:"domain,omitempty"`
	Diff         string    `json:"diff,omitempty"`
	IP           string    `json:"ip,omitempty"`
	RequestID    string    `json:"request_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// AnalyticsSummary aggregates the counters shown on the dashboard.
type AnalyticsSummary struct {
	TotalUsers          int `json:"total_users"`
	PasswordIdentities  int `json:"password_identities"`
	GoogleIdentities    int `json:"google_identities"`
	GitHubIdentities    int `json:"github_identities"`
	EnrichmentSucceeded int `json:"enrichment_succeeded"`
	EnrichmentFailed    int `json:"enrichment_failed"`
	JobsQueued          int `json:"jobs_queued"`
	JobsDone            int `json:"jobs_done"`
	JobsFailed          int `json:"jobs_failed"`
	EventsRegistered    int `json:"events_registered,omitempty"`
	EventsLoggedIn      int `json:"events_logged_in,omitempty"`
	EventsOAuthLinked   int `json:"events_oauth_linked,omitempty"`
	EventsJobsQueued    int `json:"events_jobs_queued,omitempty"`
	EventsJobsDone      int `json:"events_jobs_done,omitempty"`
	EventsJobsFailed    int `json:"events_jobs_failed,omitempty"`
}

// Setting is one stored system setting or feature flag; Value is the JSON
// literal of the definition's type (true, 365, "text").
type Setting struct {
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Description string    `json:"description"`
	UpdatedBy   string    `json:"updated_by"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Notification is one in-app notification delivered to a user through the
// bell and the notifications page, and live over SSE.
type Notification struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Link      string     `json:"link"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// EmailToken is a single-use, hashed token for password resets and email
// verification. The raw token exists only in the email body.
type EmailToken struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Kind      string     `json:"kind"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// MFAFactor is the user's TOTP enrollment. The secret stays server-side;
// confirmed flips once the user proved they can generate valid codes.
type MFAFactor struct {
	UserID      string     `json:"user_id"`
	Secret      string     `json:"-"`
	Confirmed   bool       `json:"confirmed"`
	CreatedAt   time.Time  `json:"created_at"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// MFARecoveryCode is one hashed single-use backup code. The raw code is
// shown exactly once at enrollment.
type MFARecoveryCode struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	CodeHash  string     `json:"-"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// MFAChallenge bridges the password step and the TOTP step of a login. The
// token is handed to the client once; only its hash is stored.
type MFAChallenge struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TokenHash string    `json:"-"`
	Attempts  int       `json:"attempts"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// File is one uploaded blob's metadata; the bytes live in the file store.
// RefType/RefId attach it to an entity (e.g. "ticket" + ticket id).
type File struct {
	ID          string     `json:"id"`
	OwnerID     string     `json:"owner_id"`
	OrgID       string     `json:"org_id,omitempty"`
	Key         string     `json:"-"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	SizeBytes   int64      `json:"size_bytes"`
	Checksum    string     `json:"checksum_sha256,omitempty"`
	RefType     string     `json:"ref_type,omitempty"`
	RefID       string     `json:"ref_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

// OrgInvitation is a pending org membership offer. The raw token lives only
// in the invitation email; the store keeps its sha256 hash.
type OrgInvitation struct {
	ID         string     `json:"id"`
	OrgID      string     `json:"org_id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	InvitedBy  string     `json:"invited_by,omitempty"`
	TokenHash  string     `json:"-"`
	Status     string     `json:"status"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	AcceptedBy string     `json:"accepted_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// OrgInvitation statuses.
const (
	InvitationPending  = "pending"
	InvitationAccepted = "accepted"
	InvitationRevoked  = "revoked"
	InvitationExpired  = "expired"
)

// RefreshToken is one link in a rotation chain (family). Only the sha256 of
// the token is stored; RevokedAt is set on rotation, and a *presented*
// revoked token proves the family leaked.
type RefreshToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	FamilyID   string     `json:"family_id"`
	TokenHash  string     `json:"-"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	ReplacedBy string     `json:"replaced_by,omitempty"`
	UserAgent  string     `json:"user_agent,omitempty"`
	IP         string     `json:"ip,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// APIKey is a personal key for programmatic access. The raw secret exists
// only at creation; Prefix is safe to display, KeyHash stays in the store.
type APIKey struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	OrgID      string     `json:"org_id,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// WebhookEndpoint subscribes one URL to a set of event names ("*" = all).
// The secret signs every delivery with HMAC-SHA256.
type WebhookEndpoint struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	Secret      string    `json:"-"`
	Events      []string  `json:"events"`
	Description string    `json:"description"`
	Active      bool      `json:"active"`
	OrgID       string    `json:"org_id,omitempty"`
	CreatedBy   string    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WebhookDelivery is one recorded delivery attempt chain of an event to an
// endpoint. Attempts accumulate across retries.
type WebhookDelivery struct {
	ID                 string     `json:"id"`
	EndpointID         string     `json:"endpoint_id"`
	Event              string     `json:"event"`
	Payload            string     `json:"payload"`
	Status             string     `json:"status"`
	Attempts           int        `json:"attempts"`
	LastResponseStatus int        `json:"last_response_status"`
	LastError          string     `json:"last_error"`
	DeliveredAt        *time.Time `json:"delivered_at"`
	CreatedAt          time.Time  `json:"created_at"`
}

// Ticket lifecycle statuses. rejected and withdrawn are distinct terminal
// outcomes: rejected was refused by an approver, withdrawn was closed by
// the requester themselves.
const (
	TicketDraft     = "draft"
	TicketInReview  = "in_review"
	TicketApproved  = "approved"
	TicketCompleted = "completed"
	TicketRejected  = "rejected"
	TicketWithdrawn = "withdrawn"
)

// Ticket approval step and decision statuses.
const (
	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalRejected = "rejected"
	ApprovalSkipped  = "skipped"
)

// TicketStep is one approval step of a ticket type: the displayed stage
// name and the Casbin role whose holders may decide it.
type TicketStep struct {
	Name         string `json:"name"`
	ApproverRole string `json:"approver_role"`
}

// TicketType is a configurable kind of ticket carrying its ordered approval
// chain. OrgID "" means the type is available platform-wide.
type TicketType struct {
	ID          string       `json:"id"`
	OrgID       string       `json:"org_id,omitempty"`
	Key         string       `json:"key"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Steps       []TicketStep `json:"steps"`
	Active      bool         `json:"active"`
	CreatedBy   string       `json:"created_by,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Ticket is a request filed by a user and moved through the approval state
// machine. Number is a per-organization human-readable sequence; DecidedBy
// names whoever produced the terminal outcome (an approver for rejected,
// the requester for withdrawn).
type Ticket struct {
	ID          string     `json:"id"`
	Number      int        `json:"number"`
	OrgID       string     `json:"org_id,omitempty"`
	TypeID      string     `json:"type_id"`
	TypeKey     string     `json:"type_key"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Priority    string     `json:"priority"`
	Status      string     `json:"status"`
	CurrentStep int        `json:"current_step"`
	CreatedBy   string     `json:"created_by"`
	DecidedBy   string     `json:"decided_by,omitempty"`
	DecidedAt   *time.Time `json:"decided_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TicketApproval is one step of a ticket's approval chain with its decision.
// Steps materialize as pending rows at submit time and are decided in order.
type TicketApproval struct {
	ID           string     `json:"id"`
	TicketID     string     `json:"ticket_id"`
	StepIndex    int        `json:"step_index"`
	Name         string     `json:"name"`
	ApproverRole string     `json:"approver_role"`
	Status       string     `json:"status"`
	DecidedBy    string     `json:"decided_by,omitempty"`
	DecidedAt    *time.Time `json:"decided_at"`
	Comment      string     `json:"comment,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// TicketComment is one discussion entry attached to a ticket.
type TicketComment struct {
	ID        string    `json:"id"`
	TicketID  string    `json:"ticket_id"`
	AuthorID  string    `json:"author_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// TicketEvent is one timeline entry of a ticket: who did what, when.
type TicketEvent struct {
	ID        string    `json:"id"`
	TicketID  string    `json:"ticket_id"`
	ActorID   string    `json:"actor_id"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// TicketDecision pairs an approval decision with the ticket it belongs to,
// for the approver's decided-history view.
type TicketDecision struct {
	Approval TicketApproval `json:"approval"`
	Ticket   Ticket         `json:"ticket"`
}
