// Package repository provides storage adapters for the domain models. The
// memory implementation is the development fake; a PostgreSQL adapter lives
// behind the same method surface.
package repository

import (
	"context"
	"sync"
	"time"

	"github.com/your-org/go-template/internal/analytics"
	"github.com/your-org/go-template/internal/apikeys"
	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/auth"
	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/files"
	"github.com/your-org/go-template/internal/invitations"
	"github.com/your-org/go-template/internal/jobs"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/mfa"
	"github.com/your-org/go-template/internal/notifications"
	"github.com/your-org/go-template/internal/rbac"
	"github.com/your-org/go-template/internal/settings"
	"github.com/your-org/go-template/internal/streams"
	"github.com/your-org/go-template/internal/tickets"
	"github.com/your-org/go-template/internal/tokens"
	"github.com/your-org/go-template/internal/users"
	"github.com/your-org/go-template/internal/webhooks"
)

// sessionEntry is one live session with its sliding expiry.
type sessionEntry struct {
	userID         string
	impersonatedBy string
	expiresAt      time.Time
}

// MemoryStore is an in-process, mutex-guarded store. Data does not survive
// restarts, and separate processes never see each other's state.
type MemoryStore struct {
	mu           sync.RWMutex
	users        map[string]users.User
	passwords    map[string]string
	identities   []auth.Identity
	sessions     map[string]sessionEntry
	oauthStates  map[string]auth.OAuthState
	jobs         map[string]jobs.Job
	reports      map[string]analytics.Report
	enrichments  []enrichmentRow
	enrichSeq    int64
	events       []eventRecord
	orgs         map[string]rbac.Organization
	orgMembers   []rbac.OrgMember
	roles        map[string]rbac.Role
	rules        []rbac.PolicyRule
	ruleSeq      int64
	auditLog     []audit.Entry
	settings     map[string]settings.Setting
	notifs       []notifications.Notification
	emailTokens  []emailTokenRow
	apiKeys      []apiKeyRow
	webhooks     []webhooks.Endpoint
	deliveries   []webhooks.Delivery
	ticketTypes  map[string]tickets.TicketType
	tickets      map[string]tickets.Ticket
	ticketSeq    map[string]int
	approvals    map[string][]tickets.TicketApproval
	comments     []tickets.TicketComment
	ticketEvents []tickets.TicketEvent

	// Streams: exactly-once enrichment bookkeeping, the outbox, dead
	// letters, and API idempotency reservations.
	processedEvents map[string]streams.ApplyResult // "source\x00event_id"
	processedAt     map[string]time.Time           // "source\x00event_id" → applied time (retention sweeps)
	enrichSeqs      map[string]streamSeqs          // user id → applied seq per source
	outboxItems     []*streams.OutboxEvent
	deadLetters     []streams.DeadLetter
	idempotency     map[string]kernel.IdempotencyRecord // "user_id\x00key"

	// MFA: one factor per user, hashed recovery codes, login challenges.
	mfaFactors    map[string]mfa.Factor
	mfaCodes      []mfa.RecoveryCode
	mfaChallenges []mfa.Challenge

	// Refresh-token families for the JWT grant.
	refreshTokens []tokens.RefreshToken

	// Org membership invitations.
	invitations []invitations.Invitation

	// File metadata (blobs live in the file store adapter).
	files []files.File
}

// streamSeqs tracks the highest applied enrichment sequence per source; the
// fake mirrors the users.enrich_profile_seq / enrich_contact_seq columns.
type streamSeqs struct {
	profile int64
	contact int64
}

// apiKeyRow pairs the stored key with its hash.
type apiKeyRow struct {
	key  apikeys.Key
	hash string
}

// emailTokenRow pairs the stored token with its hash for lookup.
type emailTokenRow struct {
	token email.Token
	hash  string
}

// NewMemoryStore returns an empty store ready for use.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users:           map[string]users.User{},
		passwords:       map[string]string{},
		sessions:        map[string]sessionEntry{},
		oauthStates:     map[string]auth.OAuthState{},
		jobs:            map[string]jobs.Job{},
		reports:         map[string]analytics.Report{},
		orgs:            map[string]rbac.Organization{},
		roles:           map[string]rbac.Role{},
		settings:        map[string]settings.Setting{},
		ticketTypes:     map[string]tickets.TicketType{},
		tickets:         map[string]tickets.Ticket{},
		ticketSeq:       map[string]int{},
		approvals:       map[string][]tickets.TicketApproval{},
		processedEvents: map[string]streams.ApplyResult{},
		processedAt:     map[string]time.Time{},
		enrichSeqs:      map[string]streamSeqs{},
		idempotency:     map[string]kernel.IdempotencyRecord{},
		mfaFactors:      map[string]mfa.Factor{},
	}
}

// Ping always succeeds; the fake has no external dependencies.
func (s *MemoryStore) Ping(ctx context.Context) error { return nil }
