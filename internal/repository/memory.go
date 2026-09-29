// Package repository provides storage adapters for the domain models. The
// memory implementation is the development fake; a PostgreSQL adapter lives
// behind the same method surface.
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/your-org/go-template/internal/model"
)

// topNameCounts sorts a count map and keeps the largest n entries.
func topNameCounts(counts map[string]int64, n int) []model.NameCount {
	out := make([]model.NameCount, 0, len(counts))
	for name, count := range counts {
		out = append(out, model.NameCount{Name: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// ErrNotFound is returned when a requested entity does not exist. Handlers
// map it to HTTP 404.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when an insert would violate a uniqueness
// constraint, such as a duplicate email or an identity owned by another user.
// Handlers map it to HTTP 409.
var ErrConflict = errors.New("conflict")

// ListOptions controls every paginated listing. Mode selects the pagination
// strategy: "offset" pages by Offset, "cursor" and "keyset" continue after
// the entity id (or enrichment sequence) in After. Status and Type
// optionally narrow job listings; other entities ignore them.
type ListOptions struct {
	Query  string
	Mode   string
	Limit  int
	Offset int
	After  string
	Status string // jobs only: filter by status
	Type   string // jobs only: filter by job type
	UserID string // enrichments only: filter by user id
}

// ListUsersOptions is the historical name for user listings; it is the same
// type so handlers and stores share one option surface.
type ListUsersOptions = ListOptions

// AuditOptions filters audit listings: ListOptions carries pagination, the
// extra fields narrow by actor, action, resource type, or exact resource id.
// Subject (set by the per-user activity listing) matches everything the user
// did as actor plus every 'user' record about them.
type AuditOptions struct {
	ListOptions
	Actor      string
	Action     string
	Resource   string
	ResourceID string
	Subject    string
}

// TicketOptions narrows ticket listings: ListOptions carries pagination and
// the shared query/status/type filters, the extra fields scope by creator
// (the requester's "my" view), organization, and priority.
type TicketOptions struct {
	ListOptions
	OrgID     string
	CreatedBy string
	Priority  string
}

// DeadLetterOptions narrows dead-letter listings: ListOptions carries
// pagination, Pending filters to not-yet-replayed letters.
type DeadLetterOptions struct {
	ListOptions
	Pending bool
}

// SplitEnrichmentCursor splits an enrichment "after" cursor back into its
// (created_at, sequence) components; the enrichment table has no id column,
// so its keyset cursor carries both.
func SplitEnrichmentCursor(cursor string) (time.Time, int64, bool) {
	before, after, ok := strings.Cut(cursor, ":")
	if !ok {
		return time.Time{}, 0, false
	}
	nanos, err := strconv.ParseInt(before, 10, 64)
	if err != nil {
		return time.Time{}, 0, false
	}
	seq, err := strconv.ParseInt(after, 10, 64)
	if err != nil {
		return time.Time{}, 0, false
	}
	return time.Unix(0, nanos), seq, true
}

// sessionEntry is one live session with its sliding expiry.
type sessionEntry struct {
	userID         string
	impersonatedBy string
	expiresAt      time.Time
}

// eventRecord is one analytics event with the moment it happened.
type eventRecord struct {
	name       string
	userID     string
	at         time.Time
	properties string
}

// MemoryStore is an in-process, mutex-guarded store. Data does not survive
// restarts, and separate processes never see each other's state.
type MemoryStore struct {
	mu           sync.RWMutex
	users        map[string]model.User
	passwords    map[string]string
	identities   []model.AuthIdentity
	sessions     map[string]sessionEntry
	oauthStates  map[string]model.OAuthState
	jobs         map[string]model.Job
	reports      map[string]model.Report
	enrichments  []enrichmentRow
	enrichSeq    int64
	events       []eventRecord
	orgs         map[string]model.Organization
	orgMembers   []model.OrgMember
	roles        map[string]model.Role
	rules        []model.PolicyRule
	ruleSeq      int64
	auditLog     []model.AuditEntry
	settings     map[string]model.Setting
	notifs       []model.Notification
	emailTokens  []emailTokenRow
	apiKeys      []apiKeyRow
	webhooks     []model.WebhookEndpoint
	deliveries   []model.WebhookDelivery
	ticketTypes  map[string]model.TicketType
	tickets      map[string]model.Ticket
	ticketSeq    map[string]int
	approvals    map[string][]model.TicketApproval
	comments     []model.TicketComment
	ticketEvents []model.TicketEvent

	// Streams: exactly-once enrichment bookkeeping, the outbox, dead
	// letters, and API idempotency reservations.
	processedEvents map[string]model.StreamApplyResult // "source\x00event_id"
	enrichSeqs      map[string]streamSeqs              // user id → applied seq per source
	outboxItems     []*model.OutboxEvent
	deadLetters     []model.DeadLetter
	idempotency     map[string]model.IdempotencyRecord // "user_id\x00key"

	// MFA: one factor per user, hashed recovery codes, login challenges.
	mfaFactors    map[string]model.MFAFactor
	mfaCodes      []model.MFARecoveryCode
	mfaChallenges []model.MFAChallenge

	// Refresh-token families for the JWT grant.
	refreshTokens []model.RefreshToken

	// Org membership invitations.
	invitations []model.OrgInvitation

	// File metadata (blobs live in the file store adapter).
	files []model.File
}

// streamSeqs tracks the highest applied enrichment sequence per source; the
// fake mirrors the users.enrich_profile_seq / enrich_contact_seq columns.
type streamSeqs struct {
	profile int64
	contact int64
}

// apiKeyRow pairs the stored key with its hash.
type apiKeyRow struct {
	key  model.APIKey
	hash string
}

// emailTokenRow pairs the stored token with its hash for lookup.
type emailTokenRow struct {
	token model.EmailToken
	hash  string
}

// NewMemoryStore returns an empty store ready for use.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users:           map[string]model.User{},
		passwords:       map[string]string{},
		sessions:        map[string]sessionEntry{},
		oauthStates:     map[string]model.OAuthState{},
		jobs:            map[string]model.Job{},
		reports:         map[string]model.Report{},
		orgs:            map[string]model.Organization{},
		roles:           map[string]model.Role{},
		settings:        map[string]model.Setting{},
		ticketTypes:     map[string]model.TicketType{},
		tickets:         map[string]model.Ticket{},
		ticketSeq:       map[string]int{},
		approvals:       map[string][]model.TicketApproval{},
		processedEvents: map[string]model.StreamApplyResult{},
		enrichSeqs:      map[string]streamSeqs{},
		idempotency:     map[string]model.IdempotencyRecord{},
		mfaFactors:      map[string]model.MFAFactor{},
	}
}

// Ping always succeeds; the fake has no external dependencies.
func (s *MemoryStore) Ping(ctx context.Context) error { return nil }

// CreateUser stores a user and, when passwordHash is non-empty, the password
// credential plus the "password" identity. Emails are unique
// case-insensitively.
func (s *MemoryStore) CreateUser(ctx context.Context, user model.User, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.users {
		if strings.EqualFold(existing.Email, user.Email) {
			return ErrConflict
		}
		if user.ExternalRef != "" && existing.ExternalRef == user.ExternalRef {
			return ErrConflict
		}
	}
	s.users[user.ID] = user
	if passwordHash != "" {
		s.passwords[user.Email] = passwordHash
		s.identities = append(s.identities, model.AuthIdentity{
			UserID: user.ID, Provider: "password", ProviderSubject: strings.ToLower(user.Email), Email: user.Email, Name: user.Name, LinkedAt: time.Now(), LastLoginAt: time.Now(),
		})
	}
	return nil
}

// UpdateUser overwrites an existing user and refreshes UpdatedAt. The
// enrichment columns are stream-owned; a conflicting external_ref on another
// user is rejected, mirroring the partial unique index.
func (s *MemoryStore) UpdateUser(ctx context.Context, user model.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[user.ID]; !ok {
		return ErrNotFound
	}
	if user.ExternalRef != "" {
		for id, existing := range s.users {
			if id != user.ID && existing.ExternalRef == user.ExternalRef {
				return ErrConflict
			}
		}
	}
	user.UpdatedAt = time.Now()
	s.users[user.ID] = user
	return nil
}

// DeleteUser removes the user by id and cascades org memberships.
func (s *MemoryStore) DeleteUser(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		return ErrNotFound
	}
	delete(s.users, id)
	members := s.orgMembers[:0]
	for _, member := range s.orgMembers {
		if member.UserID != id {
			members = append(members, member)
		}
	}
	s.orgMembers = members
	return nil
}

// GetUser returns the user by id.
func (s *MemoryStore) GetUser(ctx context.Context, id string) (model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[id]
	if !ok {
		return model.User{}, ErrNotFound
	}
	return user, nil
}

// GetUserByEmail looks a user up case-insensitively and returns the stored
// password hash alongside.
func (s *MemoryStore) GetUserByEmail(ctx context.Context, email string) (model.User, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.users {
		if strings.EqualFold(user.Email, email) {
			return user, s.passwords[user.Email], nil
		}
	}
	return model.User{}, "", ErrNotFound
}

// ListUsers returns a page of users ordered newest first (id ascending as a
// tiebreaker) plus the cursor for the next page, empty at the end of the
// list. Limit is clamped to the 1..100 range.
func (s *MemoryStore) ListUsers(ctx context.Context, opts ListUsersOptions) ([]model.User, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	users := make([]model.User, 0, len(s.users))
	for _, user := range s.users {
		if opts.Query == "" || strings.Contains(strings.ToLower(user.Email+" "+user.Name), strings.ToLower(opts.Query)) {
			users = append(users, user)
		}
	}
	sort.Slice(users, func(i, j int) bool {
		if users[i].CreatedAt.Equal(users[j].CreatedAt) {
			return users[i].ID < users[j].ID
		}
		return users[i].CreatedAt.After(users[j].CreatedAt)
	})
	if opts.Limit <= 0 || opts.Limit > 100 {
		opts.Limit = 25
	}
	start := opts.Offset
	if start < 0 {
		start = 0
	}
	if opts.Mode == "cursor" || opts.Mode == "keyset" {
		start = 0
		if opts.After != "" {
			for i, user := range users {
				if user.ID == opts.After {
					start = i + 1
					break
				}
			}
		}
	}
	if start > len(users) {
		return nil, "", nil
	}
	end := start + opts.Limit
	if end > len(users) {
		end = len(users)
	}
	next := ""
	if end < len(users) {
		next = users[end-1].ID
	}
	return users[start:end], next, nil
}

// LinkIdentity attaches an external identity to a user. Relinking the same
// (provider, subject) to the same user is a no-op; a different user is a
// conflict.
func (s *MemoryStore) LinkIdentity(ctx context.Context, identity model.AuthIdentity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.identities {
		if existing.Provider == identity.Provider && existing.ProviderSubject == identity.ProviderSubject {
			if existing.UserID == identity.UserID {
				return nil
			}
			return ErrConflict
		}
	}
	identity.LinkedAt = time.Now()
	if identity.LastLoginAt.IsZero() {
		identity.LastLoginAt = identity.LinkedAt
	}
	s.identities = append(s.identities, identity)
	return nil
}

// GetIdentity returns the identity for a (provider, subject) pair.
func (s *MemoryStore) GetIdentity(ctx context.Context, provider, subject string) (model.AuthIdentity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, identity := range s.identities {
		if identity.Provider == provider && identity.ProviderSubject == subject {
			return identity, nil
		}
	}
	return model.AuthIdentity{}, ErrNotFound
}

// SaveOAuthState remembers a single-use OAuth state value with its expiry.
func (s *MemoryStore) SaveOAuthState(ctx context.Context, state model.OAuthState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.oauthStates[state.State] = state
	return nil
}

// ConsumeOAuthState removes and returns the state, or ErrNotFound when it is
// unknown or expired. A consumed state can never be used again.
func (s *MemoryStore) ConsumeOAuthState(ctx context.Context, state string, now time.Time) (model.OAuthState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	parsed, ok := s.oauthStates[state]
	if !ok {
		return model.OAuthState{}, ErrNotFound
	}
	delete(s.oauthStates, state)
	if now.After(parsed.ExpiresAt) {
		return model.OAuthState{}, ErrNotFound
	}
	return parsed, nil
}

// CountUsers returns the total number of users.
func (s *MemoryStore) CountUsers(ctx context.Context) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users)
}

// UnlinkIdentity removes the identity a user holds with the given provider.
func (s *MemoryStore) UnlinkIdentity(ctx context.Context, userID, provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.identities[:0]
	for _, identity := range s.identities {
		if identity.UserID == userID && identity.Provider == provider {
			continue
		}
		next = append(next, identity)
	}
	s.identities = next
	return nil
}

// ListIdentities returns every identity linked to the user.
func (s *MemoryStore) ListIdentities(ctx context.Context, userID string) ([]model.AuthIdentity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var identities []model.AuthIdentity
	for _, identity := range s.identities {
		if identity.UserID == userID {
			identities = append(identities, identity)
		}
	}
	return identities, nil
}

// CreateSession maps an opaque session token to a user id with a TTL.
// impersonatedBy marks an impersonation session ("" for normal logins).
func (s *MemoryStore) CreateSession(ctx context.Context, token, userID, impersonatedBy string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = sessionEntry{userID: userID, impersonatedBy: impersonatedBy, expiresAt: time.Now().Add(ttl)}
	return nil
}

// GetSession returns the user id behind a session token and slides its
// expiry forward (renewal on use). Expired tokens are removed. It reports
// the impersonator, if any.
func (s *MemoryStore) GetSession(ctx context.Context, token string, ttl time.Duration) (model.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.sessions[token]
	if !ok {
		return model.Session{}, ErrNotFound
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.sessions, token)
		return model.Session{}, ErrNotFound
	}
	entry.expiresAt = time.Now().Add(ttl)
	s.sessions[token] = entry
	return model.Session{UserID: entry.userID, ImpersonatedBy: entry.impersonatedBy}, nil
}

// DeleteSession revokes a session token.
func (s *MemoryStore) DeleteSession(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
	return nil
}

// DeleteSessionsForUser revokes every session of the user (logout-all).
func (s *MemoryStore) DeleteSessionsForUser(ctx context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, entry := range s.sessions {
		if entry.userID == userID {
			delete(s.sessions, token)
		}
	}
	return nil
}

// SetPasswordCredential upserts the stored password hash; "" removes the
// credential entirely.
func (s *MemoryStore) SetPasswordCredential(ctx context.Context, userID, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return ErrNotFound
	}
	if passwordHash == "" {
		delete(s.passwords, user.Email)
		return nil
	}
	s.passwords[user.Email] = passwordHash
	return nil
}

// CreateEmailToken stores a hashed single-use token.
func (s *MemoryStore) CreateEmailToken(ctx context.Context, token model.EmailToken, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now()
	}
	s.emailTokens = append(s.emailTokens, emailTokenRow{token: token, hash: tokenHash})
	return nil
}

// GetEmailTokenByHash resolves a token by its hash.
func (s *MemoryStore) GetEmailTokenByHash(ctx context.Context, tokenHash string) (model.EmailToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, row := range s.emailTokens {
		if row.hash == tokenHash {
			return row.token, nil
		}
	}
	return model.EmailToken{}, ErrNotFound
}

// MarkEmailTokenUsed consumes the token.
func (s *MemoryStore) MarkEmailTokenUsed(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, row := range s.emailTokens {
		if row.token.ID == id {
			if row.token.UsedAt != nil {
				return ErrNotFound
			}
			now := time.Now()
			s.emailTokens[i].token.UsedAt = &now
			return nil
		}
	}
	return ErrNotFound
}

// CreateAPIKey stores a key with its hash.
func (s *MemoryStore) CreateAPIKey(ctx context.Context, key model.APIKey, keyHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key.CreatedAt.IsZero() {
		key.CreatedAt = time.Now()
	}
	s.apiKeys = append(s.apiKeys, apiKeyRow{key: key, hash: keyHash})
	return nil
}

// ListAPIKeysForUser returns the user's keys newest first.
func (s *MemoryStore) ListAPIKeysForUser(ctx context.Context, userID string) ([]model.APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.APIKey
	for _, row := range s.apiKeys {
		if row.key.UserID == userID {
			out = append(out, row.key)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// GetAPIKeyByPrefix resolves a live key by prefix and returns its hash.
func (s *MemoryStore) GetAPIKeyByPrefix(ctx context.Context, prefix string) (model.APIKey, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, row := range s.apiKeys {
		if row.key.Prefix == prefix && row.key.RevokedAt == nil {
			return row.key, row.hash, nil
		}
	}
	return model.APIKey{}, "", ErrNotFound
}

// TouchAPIKey stamps last use.
func (s *MemoryStore) TouchAPIKey(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, row := range s.apiKeys {
		if row.key.ID == id {
			now := time.Now()
			s.apiKeys[i].key.LastUsedAt = &now
		}
	}
	return nil
}

// RevokeAPIKey marks the key revoked.
func (s *MemoryStore) RevokeAPIKey(ctx context.Context, id, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, row := range s.apiKeys {
		if row.key.ID == id && row.key.UserID == userID {
			if row.key.RevokedAt != nil {
				return ErrNotFound
			}
			now := time.Now()
			s.apiKeys[i].key.RevokedAt = &now
			return nil
		}
	}
	return ErrNotFound
}

// InsertWebhookEndpoint stores an endpoint.
func (s *MemoryStore) InsertWebhookEndpoint(ctx context.Context, endpoint model.WebhookEndpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.webhooks = append(s.webhooks, endpoint)
	return nil
}

// UpdateWebhookEndpoint overwrites the endpoint.
func (s *MemoryStore) UpdateWebhookEndpoint(ctx context.Context, endpoint model.WebhookEndpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.webhooks {
		if existing.ID == endpoint.ID {
			s.webhooks[i] = endpoint
			return nil
		}
	}
	return ErrNotFound
}

// DeleteWebhookEndpoint removes the endpoint.
func (s *MemoryStore) DeleteWebhookEndpoint(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.webhooks {
		if existing.ID == id {
			s.webhooks = append(s.webhooks[:i], s.webhooks[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

// GetWebhookEndpoint returns one endpoint.
func (s *MemoryStore) GetWebhookEndpoint(ctx context.Context, id string) (model.WebhookEndpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, existing := range s.webhooks {
		if existing.ID == id {
			return existing, nil
		}
	}
	return model.WebhookEndpoint{}, ErrNotFound
}

// ListWebhookEndpoints returns every endpoint.
func (s *MemoryStore) ListWebhookEndpoints(ctx context.Context) ([]model.WebhookEndpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.WebhookEndpoint, len(s.webhooks))
	copy(out, s.webhooks)
	return out, nil
}

// InsertWebhookDelivery records a pending delivery.
func (s *MemoryStore) InsertWebhookDelivery(ctx context.Context, delivery model.WebhookDelivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = time.Now()
	}
	s.deliveries = append(s.deliveries, delivery)
	return nil
}

// UpdateWebhookDelivery rewrites the delivery attempt data.
func (s *MemoryStore) UpdateWebhookDelivery(ctx context.Context, delivery model.WebhookDelivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.deliveries {
		if existing.ID == delivery.ID {
			delivery.CreatedAt = existing.CreatedAt
			s.deliveries[i] = delivery
			return nil
		}
	}
	return ErrNotFound
}

// ListWebhookDeliveries pages the endpoint's delivery log newest first.
func (s *MemoryStore) ListWebhookDeliveries(ctx context.Context, endpointID string, limit, offset int) ([]model.WebhookDelivery, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rows []model.WebhookDelivery
	for _, delivery := range s.deliveries {
		if delivery.EndpointID == endpointID {
			rows = append(rows, delivery)
		}
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	start := max(offset, 0)
	if start >= len(rows) {
		return nil, nil
	}
	end := min(start+limit, len(rows))
	return rows[start:end], nil
}

// --- Ticket types -------------------------------------------------------

// CreateTicketType stores a ticket type; (org_id, key) is unique.
func (s *MemoryStore) CreateTicketType(ctx context.Context, typ model.TicketType) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.ticketTypes {
		if existing.OrgID == typ.OrgID && existing.Key == typ.Key {
			return ErrConflict
		}
	}
	s.ticketTypes[typ.ID] = typ
	return nil
}

// GetTicketType returns the type by id.
func (s *MemoryStore) GetTicketType(ctx context.Context, id string) (model.TicketType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	typ, ok := s.ticketTypes[id]
	if !ok {
		return model.TicketType{}, ErrNotFound
	}
	return typ, nil
}

// GetTicketTypeByKey resolves a type in one organization's namespace.
func (s *MemoryStore) GetTicketTypeByKey(ctx context.Context, orgID, key string) (model.TicketType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, typ := range s.ticketTypes {
		if typ.OrgID == orgID && typ.Key == key {
			return typ, nil
		}
	}
	return model.TicketType{}, ErrNotFound
}

// UpdateTicketType overwrites the type.
func (s *MemoryStore) UpdateTicketType(ctx context.Context, typ model.TicketType) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ticketTypes[typ.ID]; !ok {
		return ErrNotFound
	}
	s.ticketTypes[typ.ID] = typ
	return nil
}

// DeleteTicketType removes the type by id.
func (s *MemoryStore) DeleteTicketType(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ticketTypes[id]; !ok {
		return ErrNotFound
	}
	delete(s.ticketTypes, id)
	return nil
}

// ListTicketTypes returns the platform-wide types plus the organization's
// own, ordered by key.
func (s *MemoryStore) ListTicketTypes(ctx context.Context, orgID string) ([]model.TicketType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.TicketType, 0, len(s.ticketTypes))
	for _, typ := range s.ticketTypes {
		if typ.OrgID == "" || typ.OrgID == orgID {
			out = append(out, typ)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// --- Tickets ------------------------------------------------------------

// CreateTicket stores the ticket and assigns its per-organization sequence
// number, mirroring the SQL insert's number subselect.
func (s *MemoryStore) CreateTicket(ctx context.Context, ticket model.Ticket) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ticketTypes[ticket.TypeID]; !ok {
		return 0, ErrNotFound
	}
	if ticket.CreatedAt.IsZero() {
		ticket.CreatedAt = time.Now()
	}
	if ticket.UpdatedAt.IsZero() {
		ticket.UpdatedAt = ticket.CreatedAt
	}
	s.ticketSeq[ticket.OrgID]++
	ticket.Number = s.ticketSeq[ticket.OrgID]
	s.tickets[ticket.ID] = ticket
	return ticket.Number, nil
}

// GetTicket returns the ticket by id.
func (s *MemoryStore) GetTicket(ctx context.Context, id string) (model.Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ticket, ok := s.tickets[id]
	if !ok {
		return model.Ticket{}, ErrNotFound
	}
	return ticket, nil
}

// UpdateTicket overwrites the ticket record. The per-organization number is
// store-assigned and immutable, matching the SQL UPDATE that never touches
// the column.
func (s *MemoryStore) UpdateTicket(ctx context.Context, ticket model.Ticket) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.tickets[ticket.ID]
	if !ok {
		return ErrNotFound
	}
	// The copy is by-value, so keeping the stored number is invisible to
	// the caller's struct.
	ticket.Number = existing.Number
	s.tickets[ticket.ID] = ticket
	return nil
}

// DeleteTicket removes the ticket and cascades its approvals, comments, and
// events, like the foreign keys do on PostgreSQL.
func (s *MemoryStore) DeleteTicket(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tickets[id]; !ok {
		return ErrNotFound
	}
	delete(s.tickets, id)
	delete(s.approvals, id)
	comments := s.comments[:0]
	for _, comment := range s.comments {
		if comment.TicketID != id {
			comments = append(comments, comment)
		}
	}
	s.comments = comments
	events := s.ticketEvents[:0]
	for _, event := range s.ticketEvents {
		if event.TicketID != id {
			events = append(events, event)
		}
	}
	s.ticketEvents = events
	return nil
}

// ticketInScope applies the ticket listing filters. Caller holds the lock.
func ticketInScope(ticket model.Ticket, opts TicketOptions) bool {
	query := strings.ToLower(opts.Query)
	if opts.OrgID != "" && ticket.OrgID != opts.OrgID {
		return false
	}
	if opts.CreatedBy != "" && ticket.CreatedBy != opts.CreatedBy {
		return false
	}
	if opts.Status != "" && ticket.Status != opts.Status {
		return false
	}
	if opts.Type != "" && ticket.TypeKey != opts.Type {
		return false
	}
	if opts.Priority != "" && ticket.Priority != opts.Priority {
		return false
	}
	if query != "" && !strings.Contains(strings.ToLower(ticket.Title+" "+ticket.Description), query) {
		return false
	}
	return true
}

// ListTickets returns one page of tickets newest first plus the next-page
// cursor.
func (s *MemoryStore) ListTickets(ctx context.Context, opts TicketOptions) ([]model.Ticket, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := make([]model.Ticket, 0, len(s.tickets))
	for _, ticket := range s.tickets {
		if ticketInScope(ticket, opts) {
			rows = append(rows, ticket)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	page, next := pageSlice(rows, func(ticket model.Ticket) string { return ticket.ID }, opts.ListOptions)
	return page, next, nil
}

// CountTicketsByStatus counts the scoped tickets per status.
func (s *MemoryStore) CountTicketsByStatus(ctx context.Context, orgID, createdBy string) (map[string]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := map[string]int64{}
	for _, ticket := range s.tickets {
		if orgID != "" && ticket.OrgID != orgID {
			continue
		}
		if createdBy != "" && ticket.CreatedBy != createdBy {
			continue
		}
		counts[ticket.Status]++
	}
	return counts, nil
}

// --- Approval steps -----------------------------------------------------

// ReplaceApprovals rewrites the ticket's approval steps; an empty slice
// clears them (reopen).
func (s *MemoryStore) ReplaceApprovals(ctx context.Context, ticketID string, steps []model.TicketApproval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tickets[ticketID]; !ok {
		return ErrNotFound
	}
	if steps == nil {
		steps = []model.TicketApproval{}
	}
	s.approvals[ticketID] = steps
	return nil
}

// ListApprovals returns the ticket's steps in order.
func (s *MemoryStore) ListApprovals(ctx context.Context, ticketID string) ([]model.TicketApproval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	steps := s.approvals[ticketID]
	out := make([]model.TicketApproval, len(steps))
	copy(out, steps)
	return out, nil
}

// UpdateApproval rewrites one step's decision data.
func (s *MemoryStore) UpdateApproval(ctx context.Context, approval model.TicketApproval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, step := range s.approvals[approval.TicketID] {
		if step.ID == approval.ID {
			s.approvals[approval.TicketID][i] = approval
			return nil
		}
	}
	return ErrNotFound
}

// ListTicketsForApprover pages in-review tickets whose current pending step
// matches one of the given roles.
func (s *MemoryStore) ListTicketsForApprover(ctx context.Context, orgID string, roles []string, opts ListOptions) ([]model.Ticket, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	roleSet := make(map[string]bool, len(roles))
	for _, role := range roles {
		roleSet[role] = true
	}
	query := strings.ToLower(opts.Query)
	rows := make([]model.Ticket, 0, len(s.tickets))
	for _, ticket := range s.tickets {
		if ticket.Status != model.TicketInReview {
			continue
		}
		if orgID != "" && ticket.OrgID != orgID {
			continue
		}
		steps := s.approvals[ticket.ID]
		if ticket.CurrentStep >= len(steps) {
			continue
		}
		current := steps[ticket.CurrentStep]
		if current.Status != model.ApprovalPending || !roleSet[current.ApproverRole] {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(ticket.Title+" "+ticket.Description), query) {
			continue
		}
		rows = append(rows, ticket)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	page, next := pageSlice(rows, func(ticket model.Ticket) string { return ticket.ID }, opts)
	return page, next, nil
}

// CountTicketsForApprover counts the tickets waiting on the given roles.
func (s *MemoryStore) CountTicketsForApprover(ctx context.Context, orgID string, roles []string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	roleSet := make(map[string]bool, len(roles))
	for _, role := range roles {
		roleSet[role] = true
	}
	var n int64
	for _, ticket := range s.tickets {
		if ticket.Status != model.TicketInReview {
			continue
		}
		if orgID != "" && ticket.OrgID != orgID {
			continue
		}
		steps := s.approvals[ticket.ID]
		if ticket.CurrentStep < len(steps) && steps[ticket.CurrentStep].Status == model.ApprovalPending && roleSet[steps[ticket.CurrentStep].ApproverRole] {
			n++
		}
	}
	return n, nil
}

// ListDecidedApprovals returns the user's past decisions paired with their
// tickets, newest decision first.
func (s *MemoryStore) ListDecidedApprovals(ctx context.Context, userID string, limit, offset int) ([]model.TicketDecision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rows []model.TicketDecision
	for ticketID, steps := range s.approvals {
		ticket, ok := s.tickets[ticketID]
		if !ok {
			continue
		}
		for _, step := range steps {
			if step.DecidedBy == userID && !step.DecidedAt.IsZero() {
				rows = append(rows, model.TicketDecision{Approval: step, Ticket: ticket})
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i].Approval.DecidedAt, rows[j].Approval.DecidedAt
		if a.Equal(*b) {
			return rows[i].Approval.ID < rows[j].Approval.ID
		}
		return a.After(*b)
	})
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	start := max(offset, 0)
	if start >= len(rows) {
		return nil, nil
	}
	end := min(start+limit, len(rows))
	return rows[start:end], nil
}

// --- Comments and timeline ----------------------------------------------

// AddTicketComment appends one comment.
func (s *MemoryStore) AddTicketComment(ctx context.Context, comment model.TicketComment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if comment.CreatedAt.IsZero() {
		comment.CreatedAt = time.Now()
	}
	s.comments = append(s.comments, comment)
	return nil
}

// ListTicketComments pages the ticket's comments newest first.
func (s *MemoryStore) ListTicketComments(ctx context.Context, ticketID string, limit, offset int) ([]model.TicketComment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rows []model.TicketComment
	for _, comment := range s.comments {
		if comment.TicketID == ticketID {
			rows = append(rows, comment)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	start := max(offset, 0)
	if start >= len(rows) {
		return nil, nil
	}
	end := min(start+limit, len(rows))
	return rows[start:end], nil
}

// AddTicketEvent appends one timeline entry.
func (s *MemoryStore) AddTicketEvent(ctx context.Context, event model.TicketEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	s.ticketEvents = append(s.ticketEvents, event)
	return nil
}

// ListTicketEvents pages the ticket's timeline newest first.
func (s *MemoryStore) ListTicketEvents(ctx context.Context, ticketID string, limit, offset int) ([]model.TicketEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rows []model.TicketEvent
	for _, event := range s.ticketEvents {
		if event.TicketID == ticketID {
			rows = append(rows, event)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	start := max(offset, 0)
	if start >= len(rows) {
		return nil, nil
	}
	end := min(start+limit, len(rows))
	return rows[start:end], nil
}

// SaveJob upserts a job by id.
func (s *MemoryStore) SaveJob(ctx context.Context, job model.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	return nil
}

// GetJob returns a job by id.
func (s *MemoryStore) GetJob(ctx context.Context, id string) (model.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return model.Job{}, ErrNotFound
	}
	return job, nil
}

// listJobsLocked returns the jobs matching opts, newest first with the id as
// tiebreaker, and the next-page cursor (empty on the last page). Caller holds
// the lock.
func (s *MemoryStore) listJobsLocked(opts ListOptions) ([]model.Job, string) {
	jobs := make([]model.Job, 0, len(s.jobs))
	query := strings.ToLower(opts.Query)
	for _, job := range s.jobs {
		if opts.Status != "" && job.Status != opts.Status {
			continue
		}
		if opts.Type != "" && job.Type != opts.Type {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(job.Type+" "+job.Error), query) {
			continue
		}
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].CreatedAt.Equal(jobs[j].CreatedAt) {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})
	rows, next := pageSlice(jobs, func(job model.Job) string { return job.ID }, opts)
	return rows, next
}

// ListJobs returns one page of jobs plus the next-page cursor.
func (s *MemoryStore) ListJobs(ctx context.Context, opts ListOptions) ([]model.Job, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, next := s.listJobsLocked(opts)
	return rows, next, nil
}

// listReportsLocked mirrors listJobsLocked for reports. Caller holds the lock.
func (s *MemoryStore) listReportsLocked(opts ListOptions) ([]model.Report, string) {
	reports := make([]model.Report, 0, len(s.reports))
	query := strings.ToLower(opts.Query)
	for _, report := range s.reports {
		if query != "" && !strings.Contains(strings.ToLower(report.Status+" "+report.Result), query) {
			continue
		}
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].CreatedAt.Equal(reports[j].CreatedAt) {
			return reports[i].ID < reports[j].ID
		}
		return reports[i].CreatedAt.After(reports[j].CreatedAt)
	})
	rows, next := pageSlice(reports, func(report model.Report) string { return report.ID }, opts)
	return rows, next
}

// ListReports returns one page of reports plus the next-page cursor.
func (s *MemoryStore) ListReports(ctx context.Context, opts ListOptions) ([]model.Report, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, next := s.listReportsLocked(opts)
	return rows, next, nil
}

// pageSlice applies the shared pagination strategy to an already sorted
// slice: offset pages by Offset, cursor/keyset resume after the cursor the
// key function returned. Limit is clamped to 1..100 with a 25 default.
func pageSlice[T any](rows []T, key func(T) string, opts ListOptions) ([]T, string) {
	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	start := 0
	if opts.Mode == "cursor" || opts.Mode == "keyset" {
		if opts.After != "" {
			for i, row := range rows {
				if key(row) == opts.After {
					start = i + 1
					break
				}
			}
		}
	} else {
		start = max(opts.Offset, 0)
	}
	if start >= len(rows) {
		return nil, ""
	}
	end := min(start+limit, len(rows))
	next := ""
	if end < len(rows) {
		next = key(rows[end-1])
	}
	return rows[start:end], next
}

// SaveReport upserts a report by id.
func (s *MemoryStore) SaveReport(ctx context.Context, report model.Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[report.ID] = report
	return nil
}

// GetReport returns a report by id.
func (s *MemoryStore) GetReport(ctx context.Context, id string) (model.Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report, ok := s.reports[id]
	if !ok {
		return model.Report{}, ErrNotFound
	}
	return report, nil
}

// enrichmentRow pairs a stored profile with its insertion sequence, the
// keyset tiebreaker mirroring the Postgres seq column.
type enrichmentRow struct {
	profile model.EnrichmentProfile
	seq     int64
}

// AddEnrichment appends an enrichment profile record with a monotonic
// sequence used as the keyset tiebreaker.
func (s *MemoryStore) AddEnrichment(ctx context.Context, profile model.EnrichmentProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enrichSeq++
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = time.Now()
	}
	s.enrichments = append(s.enrichments, enrichmentRow{profile: profile, seq: s.enrichSeq})
	return nil
}

// ListEnrichments returns one page of enrichment profiles newest first. The
// cursor is "<created_at unix nano>:<sequence>", shared with the Postgres
// adapter so both stores speak the same cursor dialect.
func (s *MemoryStore) ListEnrichments(ctx context.Context, opts ListOptions) ([]model.EnrichmentProfile, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := make([]enrichmentRow, 0, len(s.enrichments))
	query := strings.ToLower(opts.Query)
	for _, row := range s.enrichments {
		if opts.UserID != "" && row.profile.UserID != opts.UserID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(row.profile.Source+" "+row.profile.Payload), query) {
			continue
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].profile.CreatedAt.After(rows[j].profile.CreatedAt) })
	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	start := 0
	if opts.Mode == "cursor" || opts.Mode == "keyset" {
		if afterAt, afterSeq, ok := SplitEnrichmentCursor(opts.After); ok {
			for i, row := range rows {
				createdAt, seq := row.profile.CreatedAt, row.seq
				if createdAt.After(afterAt) || (createdAt.Equal(afterAt) && seq > afterSeq) {
					start = i
					break
				}
			}
		}
	} else {
		start = max(opts.Offset, 0)
	}
	if start >= len(rows) {
		return nil, "", nil
	}
	end := min(start+limit, len(rows))
	cursor := ""
	if end < len(rows) {
		last := rows[end-1]
		cursor = strconv.FormatInt(last.profile.CreatedAt.UnixNano(), 10) + ":" + strconv.FormatInt(last.seq, 10)
	}
	out := make([]model.EnrichmentProfile, 0, end-start)
	for _, row := range rows[start:end] {
		out = append(out, row.profile)
	}
	return out, cursor, nil
}

// ListSessions returns the user's active sessions newest first, keyed by a
// short prefix of the opaque token.
func (s *MemoryStore) ListSessions(ctx context.Context, userID string) ([]model.SessionInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var sessions []model.SessionInfo
	for token, entry := range s.sessions {
		if entry.userID != userID || now.After(entry.expiresAt) {
			continue
		}
		sessions = append(sessions, model.SessionInfo{
			ID:        tokenPrefix(token),
			CreatedAt: now, // the fake does not track creation time
			ExpiresAt: entry.expiresAt,
		})
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].ExpiresAt.After(sessions[j].ExpiresAt) })
	return sessions, nil
}

// RecentEvents returns the most recent analytics events, newest first.
func (s *MemoryStore) RecentEvents(ctx context.Context, limit int) ([]model.EventRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	out := make([]model.EventRecord, 0, limit)
	for i := len(s.events) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, model.EventRecord{Name: s.events[i].name, OccurredAt: s.events[i].at, UserID: s.events[i].userID, Properties: s.events[i].properties})
	}
	return out, nil
}

// EventCounts aggregates the local event trail by name.
func (s *MemoryStore) EventCounts(ctx context.Context) (map[string]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := map[string]int64{}
	for _, e := range s.events {
		counts[e.name]++
	}
	return counts, nil
}

// InsertEvents bulk-loads historical events (demo seed backfill); the fake
// just appends them in the given order.
func (s *MemoryStore) InsertEvents(ctx context.Context, events []model.EventRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range events {
		s.events = append(s.events, eventRecord{name: e.Name, userID: e.UserID, at: e.OccurredAt, properties: e.Properties})
	}
	return nil
}

// DeleteDemoEvents drops events attributed to demo users (demo reset).
func (s *MemoryStore) DeleteDemoEvents(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.events[:0]
	for _, e := range s.events {
		if strings.HasPrefix(e.userID, "demo-") {
			continue
		}
		next = append(next, e)
	}
	s.events = next
	return nil
}

// EventTimeseries aggregates the local event trail by calendar day.
func (s *MemoryStore) EventTimeseries(ctx context.Context, days int) ([]model.EventDayPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	since := time.Now().AddDate(0, 0, -days).Truncate(24 * time.Hour)
	byDay := map[string]map[string]int64{}
	for _, event := range s.events {
		if event.at.Before(since) {
			continue
		}
		day := event.at.Format("2006-01-02")
		counts, ok := byDay[day]
		if !ok {
			counts = map[string]int64{}
			byDay[day] = counts
		}
		counts[event.name]++
	}
	return dayPoints(byDay), nil
}

// EventBreakdown groups the window's events by one JSON property dimension.
func (s *MemoryStore) EventBreakdown(ctx context.Context, dimension string, days int) ([]model.NameCount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	since := time.Now().AddDate(0, 0, -days)
	key := fmt.Sprintf(`"%s":`, dimension)
	counts := map[string]int64{}
	for _, event := range s.events {
		if event.at.Before(since) {
			continue
		}
		var properties map[string]any
		_ = json.Unmarshal([]byte(event.properties), &properties)
		if value, ok := properties[dimension].(string); ok {
			counts[value]++
			continue
		}
		_ = key
		counts[""]++
	}
	delete(counts, "")
	return topNameCounts(counts, 10), nil
}

// DailyActiveUsers counts distinct event-attributed users per day.
func (s *MemoryStore) DailyActiveUsers(ctx context.Context, days int) ([]model.DayCount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	since := time.Now().AddDate(0, 0, -days)
	byDay := map[string]map[string]bool{}
	for _, event := range s.events {
		if event.at.Before(since) || event.userID == "" {
			continue
		}
		day := event.at.Format("2006-01-02")
		if byDay[day] == nil {
			byDay[day] = map[string]bool{}
		}
		byDay[day][event.userID] = true
	}
	days2 := make([]string, 0, len(byDay))
	for day := range byDay {
		days2 = append(days2, day)
	}
	sort.Strings(days2)
	out := make([]model.DayCount, 0, len(days2))
	for _, day := range days2 {
		out = append(out, model.DayCount{Day: day, Count: int64(len(byDay[day]))})
	}
	return out, nil
}

// AddEvent records a named analytics event tied to the acting user.
func (s *MemoryStore) AddEvent(ctx context.Context, event, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, eventRecord{name: event, userID: userID, at: time.Now()})
}

// Analytics computes the dashboard summary from the current state.
func (s *MemoryStore) Analytics(ctx context.Context) model.AnalyticsSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var summary model.AnalyticsSummary
	summary.TotalUsers = len(s.users)
	for _, identity := range s.identities {
		switch identity.Provider {
		case "password":
			summary.PasswordIdentities++
		case "google":
			summary.GoogleIdentities++
		case "github":
			summary.GitHubIdentities++
		}
	}
	for _, job := range s.jobs {
		switch job.Status {
		case "queued":
			summary.JobsQueued++
		case "done":
			summary.JobsDone++
		case "failed":
			summary.JobsFailed++
		}
	}
	summary.EnrichmentSucceeded = len(s.enrichments)
	for _, event := range s.events {
		switch event.name {
		case "user_registered":
			summary.EventsRegistered++
		case "user_logged_in":
			summary.EventsLoggedIn++
		case "oauth_linked":
			summary.EventsOAuthLinked++
		case "job_queued":
			summary.EventsJobsQueued++
		case "job_done":
			summary.EventsJobsDone++
		case "job_failed":
			summary.EventsJobsFailed++
		}
	}
	return summary
}

// CreateOrg stores an organization; ids are unique, slugs conflict.
func (s *MemoryStore) CreateOrg(ctx context.Context, org model.Organization) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.orgs {
		if existing.Slug == org.Slug {
			return ErrConflict
		}
	}
	if org.CreatedAt.IsZero() {
		org.CreatedAt = time.Now()
	}
	if org.UpdatedAt.IsZero() {
		org.UpdatedAt = org.CreatedAt
	}
	s.orgs[org.ID] = org
	return nil
}

// GetOrg returns an organization by id.
func (s *MemoryStore) GetOrg(ctx context.Context, id string) (model.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	org, ok := s.orgs[id]
	if !ok {
		return model.Organization{}, ErrNotFound
	}
	return org, nil
}

// UpdateOrg overwrites name and slug, refreshing UpdatedAt.
func (s *MemoryStore) UpdateOrg(ctx context.Context, org model.Organization) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orgs[org.ID]; !ok {
		return ErrNotFound
	}
	for id, existing := range s.orgs {
		if id != org.ID && existing.Slug == org.Slug {
			return ErrConflict
		}
	}
	org.UpdatedAt = time.Now()
	s.orgs[org.ID] = org
	return nil
}

// DeleteOrg removes the organization; memberships cascade.
func (s *MemoryStore) DeleteOrg(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orgs[id]; !ok {
		return ErrNotFound
	}
	delete(s.orgs, id)
	members := s.orgMembers[:0]
	for _, member := range s.orgMembers {
		if member.OrgID != id {
			members = append(members, member)
		}
	}
	s.orgMembers = members
	return nil
}

// ListOrgs returns a page of organizations newest first.
func (s *MemoryStore) ListOrgs(ctx context.Context, opts ListOptions) ([]model.Organization, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	orgs := make([]model.Organization, 0, len(s.orgs))
	query := strings.ToLower(opts.Query)
	for _, org := range s.orgs {
		if query != "" && !strings.Contains(strings.ToLower(org.Name+" "+org.Slug), query) {
			continue
		}
		orgs = append(orgs, org)
	}
	sort.Slice(orgs, func(i, j int) bool {
		if orgs[i].CreatedAt.Equal(orgs[j].CreatedAt) {
			return orgs[i].ID < orgs[j].ID
		}
		return orgs[i].CreatedAt.After(orgs[j].CreatedAt)
	})
	rows, next := pageSlice(orgs, func(org model.Organization) string { return org.ID }, opts)
	return rows, next, nil
}

// ListOrgsForUser returns the organizations the user belongs to, name order.
func (s *MemoryStore) ListOrgsForUser(ctx context.Context, userID string) ([]model.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var orgs []model.Organization
	for _, member := range s.orgMembers {
		if member.UserID != userID {
			continue
		}
		if org, ok := s.orgs[member.OrgID]; ok {
			orgs = append(orgs, org)
		}
	}
	sort.Slice(orgs, func(i, j int) bool { return orgs[i].Name < orgs[j].Name })
	return orgs, nil
}

// AddOrgMember records a membership; duplicates are a no-op and unknown
// organizations or users are ErrNotFound.
func (s *MemoryStore) AddOrgMember(ctx context.Context, member model.OrgMember) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orgs[member.OrgID]; !ok {
		return ErrNotFound
	}
	if _, ok := s.users[member.UserID]; !ok {
		return ErrNotFound
	}
	for _, existing := range s.orgMembers {
		if existing.OrgID == member.OrgID && existing.UserID == member.UserID {
			return nil
		}
	}
	if member.JoinedAt.IsZero() {
		member.JoinedAt = time.Now()
	}
	s.orgMembers = append(s.orgMembers, member)
	return nil
}

// RemoveOrgMember detaches the user from the organization.
func (s *MemoryStore) RemoveOrgMember(ctx context.Context, orgID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := s.orgMembers[:0]
	for _, member := range s.orgMembers {
		if member.OrgID == orgID && member.UserID == userID {
			continue
		}
		members = append(members, member)
	}
	s.orgMembers = members
	return nil
}

// IsOrgMember reports membership.
func (s *MemoryStore) IsOrgMember(ctx context.Context, orgID, userID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, member := range s.orgMembers {
		if member.OrgID == orgID && member.UserID == userID {
			return true, nil
		}
	}
	return false, nil
}

// ListOrgMembers returns the members of an organization with their user
// snapshot, oldest join first.
func (s *MemoryStore) ListOrgMembers(ctx context.Context, orgID string) ([]model.OrgMember, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.orgs[orgID]; !ok {
		return nil, ErrNotFound
	}
	var members []model.OrgMember
	for _, member := range s.orgMembers {
		if member.OrgID != orgID {
			continue
		}
		if user, ok := s.users[member.UserID]; ok {
			member.Email = user.Email
			member.Name = user.Name
		}
		members = append(members, member)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].JoinedAt.Before(members[j].JoinedAt) })
	return members, nil
}

// CreateRole stores role metadata; keys are unique, re-seeding is a no-op.
func (s *MemoryStore) CreateRole(ctx context.Context, role model.Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roles[role.Key]; ok {
		return nil
	}
	if role.CreatedAt.IsZero() {
		role.CreatedAt = time.Now()
	}
	s.roles[role.Key] = role
	return nil
}

// GetRole returns role metadata by key.
func (s *MemoryStore) GetRole(ctx context.Context, key string) (model.Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	role, ok := s.roles[key]
	if !ok {
		return model.Role{}, ErrNotFound
	}
	return role, nil
}

// UpdateRole overwrites display metadata; keys and the system flag are
// immutable.
func (s *MemoryStore) UpdateRole(ctx context.Context, role model.Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.roles[role.Key]
	if !ok {
		return ErrNotFound
	}
	existing.Name = role.Name
	existing.Description = role.Description
	s.roles[role.Key] = existing
	return nil
}

// DeleteRole removes role metadata.
func (s *MemoryStore) DeleteRole(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roles[key]; !ok {
		return ErrNotFound
	}
	delete(s.roles, key)
	return nil
}

// ListRoles returns every role, creation order.
func (s *MemoryStore) ListRoles(ctx context.Context) ([]model.Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	roles := make([]model.Role, 0, len(s.roles))
	for _, role := range s.roles {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].CreatedAt.Before(roles[j].CreatedAt) })
	return roles, nil
}

// ListPolicyRules returns stored rules of one ptype in insertion order —
// the same order the Casbin adapter loads them in.
func (s *MemoryStore) ListPolicyRules(ctx context.Context, ptype string) ([]model.PolicyRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rules []model.PolicyRule
	for _, rule := range s.rules {
		if rule.PType == ptype {
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

// InsertPolicyRule appends one rule; duplicates are a no-op so re-seeding
// stays idempotent.
func (s *MemoryStore) InsertPolicyRule(ctx context.Context, rule model.PolicyRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.rules {
		if policyRuleEqual(existing, rule) {
			return nil
		}
	}
	s.ruleSeq++
	s.rules = append(s.rules, rule)
	return nil
}

// DeletePolicyRule removes the exact rule.
func (s *MemoryStore) DeletePolicyRule(ctx context.Context, rule model.PolicyRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.rules[:0]
	for _, existing := range s.rules {
		if policyRuleEqual(existing, rule) {
			continue
		}
		next = append(next, existing)
	}
	s.rules = next
	return nil
}

// DeleteFilteredPolicyRules removes every rule of the ptype whose subject
// and domain match — replacing a role's permission set in one domain.
func (s *MemoryStore) DeleteFilteredPolicyRules(ctx context.Context, ptype, subject, domain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.rules[:0]
	for _, existing := range s.rules {
		if existing.PType == ptype && existing.Subject == subject && existing.Domain == domain {
			continue
		}
		next = append(next, existing)
	}
	s.rules = next
	return nil
}

// DeletePolicyRulesByType removes every rule of the ptype (adapter
// SavePolicy rebuild).
func (s *MemoryStore) DeletePolicyRulesByType(ctx context.Context, ptype string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.rules[:0]
	for _, existing := range s.rules {
		if existing.PType != ptype {
			next = append(next, existing)
		}
	}
	s.rules = next
	return nil
}

// RecordAudit appends one immutable audit record.
func (s *MemoryStore) RecordAudit(ctx context.Context, entry model.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	s.auditLog = append(s.auditLog, entry)
	return nil
}

// ListAudit returns the trail newest first, filtered by the options. The
// fake keeps every record; retention is a PostgreSQL concern.
func (s *MemoryStore) ListAudit(ctx context.Context, opts AuditOptions) ([]model.AuditEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows := make([]model.AuditEntry, 0, len(s.auditLog))
	for _, entry := range s.auditLog {
		if opts.Subject != "" {
			isActor := entry.ActorID == opts.Subject
			isSubject := entry.ResourceType == "user" && entry.ResourceID == opts.Subject
			if !isActor && !isSubject {
				continue
			}
		}
		if opts.Actor != "" && entry.ActorID != opts.Actor {
			continue
		}
		if opts.Action != "" && entry.Action != opts.Action {
			continue
		}
		if opts.Resource != "" && entry.ResourceType != opts.Resource {
			continue
		}
		if opts.ResourceID != "" && entry.ResourceID != opts.ResourceID {
			continue
		}
		rows = append(rows, entry)
	}
	// Newest first without reordering the append-only slice.
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	start := max(opts.Offset, 0)
	if start >= len(rows) {
		return nil, nil
	}
	end := min(start+limit, len(rows))
	return rows[start:end], nil
}

// UpsertSetting stores one setting.
func (s *MemoryStore) UpsertSetting(ctx context.Context, setting model.Setting) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[setting.Key] = setting
	return nil
}

// GetSetting returns one setting by key.
func (s *MemoryStore) GetSetting(ctx context.Context, key string) (model.Setting, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	setting, ok := s.settings[key]
	if !ok {
		return model.Setting{}, ErrNotFound
	}
	return setting, nil
}

// ListSettings returns every setting ordered by key.
func (s *MemoryStore) ListSettings(ctx context.Context) ([]model.Setting, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Setting, 0, len(s.settings))
	for _, setting := range s.settings {
		out = append(out, setting)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// CreateNotification appends one notification.
func (s *MemoryStore) CreateNotification(ctx context.Context, notification model.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now()
	}
	s.notifs = append(s.notifs, notification)
	return nil
}

// ListNotifications returns the user's notifications newest first.
func (s *MemoryStore) ListNotifications(ctx context.Context, userID string, limit, offset int) ([]model.Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rows []model.Notification
	for _, n := range s.notifs {
		if n.UserID == userID {
			rows = append(rows, n)
		}
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	start := max(offset, 0)
	if start >= len(rows) {
		return nil, nil
	}
	end := min(start+limit, len(rows))
	return rows[start:end], nil
}

// CountUnreadNotifications returns the user's unread count.
func (s *MemoryStore) CountUnreadNotifications(ctx context.Context, userID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var n int64
	for _, notification := range s.notifs {
		if notification.UserID == userID && notification.ReadAt == nil {
			n++
		}
	}
	return n, nil
}

// MarkNotificationRead stamps one notification as read.
func (s *MemoryStore) MarkNotificationRead(ctx context.Context, id, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, notification := range s.notifs {
		if notification.ID == id && notification.UserID == userID {
			if notification.ReadAt == nil {
				now := time.Now()
				s.notifs[i].ReadAt = &now
			}
			return nil
		}
	}
	return ErrNotFound
}

// MarkAllNotificationsRead stamps every unread notification of the user.
func (s *MemoryStore) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i, notification := range s.notifs {
		if notification.UserID == userID && notification.ReadAt == nil {
			s.notifs[i].ReadAt = &now
		}
	}
	return nil
}

// GetNotification returns one notification by id.
func (s *MemoryStore) GetNotification(ctx context.Context, id string) (model.Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, notification := range s.notifs {
		if notification.ID == id {
			return notification, nil
		}
	}
	return model.Notification{}, ErrNotFound
}

// DeleteDemoNotifications drops demo-seeded notifications (demo reset).
func (s *MemoryStore) DeleteDemoNotifications(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.notifs[:0]
	for _, notification := range s.notifs {
		if strings.HasPrefix(notification.ID, "demo-") || strings.HasPrefix(notification.UserID, "demo-") {
			continue
		}
		next = append(next, notification)
	}
	s.notifs = next
	return nil
}

// DeleteDemoGroupRules drops demo role assignments (demo reset).
func (s *MemoryStore) DeleteDemoGroupRules(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.rules[:0]
	for _, rule := range s.rules {
		if rule.PType == "g" && strings.HasPrefix(rule.Subject, "demo-") {
			continue
		}
		next = append(next, rule)
	}
	s.rules = next
	return nil
}

// DeleteDemoOrgRules drops demo org-scoped policies (demo reset).
func (s *MemoryStore) DeleteDemoOrgRules(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.rules[:0]
	for _, rule := range s.rules {
		if rule.PType == "p" && strings.HasPrefix(rule.Domain, "demo-") {
			continue
		}
		next = append(next, rule)
	}
	s.rules = next
	return nil
}

// policyRuleEqual compares the five significant rule columns.
func policyRuleEqual(a, b model.PolicyRule) bool {
	return a.PType == b.PType && a.Subject == b.Subject && a.Domain == b.Domain &&
		a.Object == b.Object && a.Action == b.Action
}

// ResetDemoData drops demo-seeded entities (bootstrap reset extension).
func (s *MemoryStore) ResetDemoData(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.users {
		if strings.HasPrefix(id, "demo-") {
			delete(s.users, id)
		}
	}
	for id := range s.jobs {
		if strings.HasPrefix(id, "demo-") {
			delete(s.jobs, id)
		}
	}
	for id := range s.reports {
		if strings.HasPrefix(id, "demo-") {
			delete(s.reports, id)
		}
	}
	for id := range s.orgs {
		if strings.HasPrefix(id, "demo-") {
			delete(s.orgs, id)
		}
	}
	members := s.orgMembers[:0]
	for _, member := range s.orgMembers {
		if strings.HasPrefix(member.OrgID, "demo-") || strings.HasPrefix(member.UserID, "demo-") {
			continue
		}
		members = append(members, member)
	}
	s.orgMembers = members
	s.enrichments = nil
	s.enrichSeq = 0
	s.emailTokens = nil
	endpoints := s.webhooks[:0]
	for _, endpoint := range s.webhooks {
		if !strings.HasPrefix(endpoint.ID, "demo-") {
			endpoints = append(endpoints, endpoint)
		}
	}
	s.webhooks = endpoints
	pending := s.deliveries[:0]
	for _, delivery := range s.deliveries {
		if strings.HasPrefix(delivery.ID, "demo-") || strings.HasPrefix(delivery.EndpointID, "demo-") {
			continue
		}
		pending = append(pending, delivery)
	}
	s.deliveries = pending
	// Demo tickets plus anything referencing the seeded ticket types — the
	// foreign keys enforce this on PostgreSQL, the fake mirrors it here.
	for id, ticket := range s.tickets {
		if strings.HasPrefix(id, "demo-") || strings.HasPrefix(ticket.TypeID, "demo-") {
			delete(s.tickets, id)
			delete(s.approvals, id)
		}
	}
	for id := range s.ticketTypes {
		if strings.HasPrefix(id, "demo-") {
			delete(s.ticketTypes, id)
		}
	}
	comments := s.comments[:0]
	for _, comment := range s.comments {
		if strings.HasPrefix(comment.TicketID, "demo-") || strings.HasPrefix(comment.AuthorID, "demo-") {
			continue
		}
		comments = append(comments, comment)
	}
	s.comments = comments
	events := s.ticketEvents[:0]
	for _, event := range s.ticketEvents {
		if strings.HasPrefix(event.TicketID, "demo-") || strings.HasPrefix(event.ActorID, "demo-") {
			continue
		}
		events = append(events, event)
	}
	s.ticketEvents = events
	for key, applied := range s.processedEvents {
		if strings.HasPrefix(applied.UserID, "demo-") {
			delete(s.processedEvents, key)
		}
	}
	for id := range s.enrichSeqs {
		if strings.HasPrefix(id, "demo-") {
			delete(s.enrichSeqs, id)
		}
	}
	outbox := s.outboxItems[:0]
	for _, item := range s.outboxItems {
		if !strings.HasPrefix(item.AggregateID, "demo-") {
			outbox = append(outbox, item)
		}
	}
	s.outboxItems = outbox
	return nil
}

// tokenPrefix derives the public session id: a short prefix of the opaque
// token, enough to tell sessions apart and useless to resume one.
func tokenPrefix(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}

// dayPoints converts a day→event→count map into the sorted response shape.
func dayPoints(byDay map[string]map[string]int64) []model.EventDayPoint {
	days := make([]string, 0, len(byDay))
	for day := range byDay {
		days = append(days, day)
	}
	sort.Strings(days)
	points := make([]model.EventDayPoint, 0, len(days))
	for _, day := range days {
		counts := byDay[day]
		var total int64
		for _, n := range counts {
			total += n
		}
		points = append(points, model.EventDayPoint{Day: day, Events: counts, Total: total})
	}
	return points
}

// --- MFA: TOTP factors, recovery codes, login challenges -------------------

func (s *MemoryStore) UpsertMFAFactor(_ context.Context, factor model.MFAFactor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mfaFactors[factor.UserID] = factor
	return nil
}

func (s *MemoryStore) ConfirmMFAFactor(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	factor, ok := s.mfaFactors[userID]
	if !ok {
		return fmt.Errorf("%w: no pending factor", ErrNotFound)
	}
	now := time.Now()
	factor.Confirmed = true
	factor.ConfirmedAt = &now
	factor.UpdatedAt = now
	s.mfaFactors[userID] = factor
	return nil
}

func (s *MemoryStore) GetMFAFactor(_ context.Context, userID string) (model.MFAFactor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	factor, ok := s.mfaFactors[userID]
	if !ok {
		return model.MFAFactor{}, fmt.Errorf("%w: no factor", ErrNotFound)
	}
	return factor, nil
}

func (s *MemoryStore) DeleteMFAFactor(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.mfaFactors, userID)
	return nil
}

func (s *MemoryStore) CreateMFARecoveryCode(_ context.Context, code model.MFARecoveryCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mfaCodes = append(s.mfaCodes, code)
	return nil
}

func (s *MemoryStore) ListMFARecoveryCodes(_ context.Context, userID string) ([]model.MFARecoveryCode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.MFARecoveryCode
	for _, code := range s.mfaCodes {
		if code.UserID == userID {
			out = append(out, code)
		}
	}
	return out, nil
}

func (s *MemoryStore) ConsumeMFARecoveryCode(_ context.Context, userID, codeHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, code := range s.mfaCodes {
		if code.UserID == userID && code.CodeHash == codeHash && code.UsedAt == nil {
			now := time.Now()
			s.mfaCodes[i].UsedAt = &now
			return nil
		}
	}
	return fmt.Errorf("%w: code not found or used", ErrNotFound)
}

func (s *MemoryStore) DeleteMFARecoveryCodes(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	codes := s.mfaCodes[:0]
	for _, code := range s.mfaCodes {
		if code.UserID != userID {
			codes = append(codes, code)
		}
	}
	s.mfaCodes = codes
	return nil
}

func (s *MemoryStore) CreateMFAChallenge(_ context.Context, challenge model.MFAChallenge) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mfaChallenges = append(s.mfaChallenges, challenge)
	return nil
}

func (s *MemoryStore) GetMFAChallengeByHash(_ context.Context, tokenHash string) (model.MFAChallenge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, challenge := range s.mfaChallenges {
		if challenge.TokenHash == tokenHash {
			return challenge, nil
		}
	}
	return model.MFAChallenge{}, fmt.Errorf("%w: challenge not found", ErrNotFound)
}

func (s *MemoryStore) IncrementMFAChallengeAttempts(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, challenge := range s.mfaChallenges {
		if challenge.ID == id {
			s.mfaChallenges[i].Attempts++
			return nil
		}
	}
	return fmt.Errorf("%w: challenge not found", ErrNotFound)
}

func (s *MemoryStore) DeleteMFAChallenge(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	challenges := s.mfaChallenges[:0]
	for _, challenge := range s.mfaChallenges {
		if challenge.ID != id {
			challenges = append(challenges, challenge)
		}
	}
	s.mfaChallenges = challenges
	return nil
}

func (s *MemoryStore) DeleteExpiredMFAChallenges(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	challenges := s.mfaChallenges[:0]
	for _, challenge := range s.mfaChallenges {
		if challenge.ExpiresAt.After(now) {
			challenges = append(challenges, challenge)
		}
	}
	s.mfaChallenges = challenges
	return nil
}

// --- Refresh tokens: JWT grant rotation families ---------------------------

func (s *MemoryStore) CreateRefreshToken(_ context.Context, token model.RefreshToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshTokens = append(s.refreshTokens, token)
	return nil
}

func (s *MemoryStore) GetRefreshTokenByHash(_ context.Context, tokenHash string) (model.RefreshToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, token := range s.refreshTokens {
		if token.TokenHash == tokenHash {
			return token, nil
		}
	}
	return model.RefreshToken{}, fmt.Errorf("%w: refresh token not found", ErrNotFound)
}

func (s *MemoryStore) RevokeRefreshToken(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, token := range s.refreshTokens {
		if token.ID == id {
			if s.refreshTokens[i].RevokedAt == nil {
				now := time.Now()
				s.refreshTokens[i].RevokedAt = &now
			}
			return nil
		}
	}
	return fmt.Errorf("%w: refresh token not found", ErrNotFound)
}

func (s *MemoryStore) RevokeRefreshFamily(_ context.Context, familyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i, token := range s.refreshTokens {
		if token.FamilyID == familyID && s.refreshTokens[i].RevokedAt == nil {
			s.refreshTokens[i].RevokedAt = &now
		}
	}
	return nil
}

func (s *MemoryStore) RevokeRefreshTokensForUser(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i, token := range s.refreshTokens {
		if token.UserID == userID && s.refreshTokens[i].RevokedAt == nil {
			s.refreshTokens[i].RevokedAt = &now
		}
	}
	return nil
}

func (s *MemoryStore) DeleteExpiredRefreshTokens(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	deadline := time.Now().Add(-7 * 24 * time.Hour)
	kept := s.refreshTokens[:0]
	for _, token := range s.refreshTokens {
		if token.ExpiresAt.After(deadline) {
			kept = append(kept, token)
		}
	}
	s.refreshTokens = kept
	return nil
}

// --- Full-text search (substring stand-in for tsvector) --------------------

func (s *MemoryStore) SearchUsers(_ context.Context, query string, limit int) ([]model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	needle := strings.ToLower(query)
	out := make([]model.User, 0, limit)
	for _, user := range s.users {
		if len(out) >= limit {
			break
		}
		if strings.Contains(strings.ToLower(user.Name), needle) || strings.Contains(strings.ToLower(user.Email), needle) {
			out = append(out, user)
		}
	}
	return out, nil
}

func (s *MemoryStore) SearchTickets(_ context.Context, domain, query string, limit int) ([]model.Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	needle := strings.ToLower(query)
	out := make([]model.Ticket, 0, limit)
	for _, ticket := range s.tickets {
		if len(out) >= limit {
			break
		}
		if domain != "" && domain != "*" && ticket.OrgID != domain {
			continue
		}
		if strings.Contains(strings.ToLower(ticket.Title), needle) || strings.Contains(strings.ToLower(ticket.Description), needle) {
			out = append(out, ticket)
		}
	}
	return out, nil
}

// --- Org invitations --------------------------------------------------------

func (s *MemoryStore) CreateOrgInvitation(_ context.Context, invitation model.OrgInvitation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invitations = append(s.invitations, invitation)
	return nil
}

func (s *MemoryStore) GetOrgInvitationByHash(_ context.Context, tokenHash string) (model.OrgInvitation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, invitation := range s.invitations {
		if invitation.TokenHash == tokenHash {
			return invitation, nil
		}
	}
	return model.OrgInvitation{}, fmt.Errorf("%w: invitation not found", ErrNotFound)
}

func (s *MemoryStore) GetOrgInvitation(_ context.Context, id string) (model.OrgInvitation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, invitation := range s.invitations {
		if invitation.ID == id {
			return invitation, nil
		}
	}
	return model.OrgInvitation{}, fmt.Errorf("%w: invitation not found", ErrNotFound)
}

func (s *MemoryStore) ListOrgInvitations(_ context.Context, orgID string, limit, offset int) ([]model.OrgInvitation, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var orgInvitations []model.OrgInvitation
	for _, invitation := range s.invitations {
		if invitation.OrgID == orgID {
			orgInvitations = append(orgInvitations, invitation)
		}
	}
	total := int64(len(orgInvitations))
	if offset >= len(orgInvitations) {
		return []model.OrgInvitation{}, total, nil
	}
	end := offset + limit
	if end > len(orgInvitations) {
		end = len(orgInvitations)
	}
	return orgInvitations[offset:end], total, nil
}

func (s *MemoryStore) ListPendingOrgInvitationsByEmail(_ context.Context, email string) ([]model.OrgInvitation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var out []model.OrgInvitation
	for _, invitation := range s.invitations {
		if strings.EqualFold(invitation.Email, email) && invitation.Status == "pending" && invitation.ExpiresAt.After(now) {
			out = append(out, invitation)
		}
	}
	return out, nil
}

func (s *MemoryStore) SetOrgInvitationStatus(_ context.Context, id, status string, acceptedAt *time.Time, acceptedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, invitation := range s.invitations {
		if invitation.ID == id {
			s.invitations[i].Status = status
			if acceptedAt != nil {
				s.invitations[i].AcceptedAt = acceptedAt
			}
			if acceptedBy != "" {
				s.invitations[i].AcceptedBy = acceptedBy
			}
			return nil
		}
	}
	return fmt.Errorf("%w: invitation not found", ErrNotFound)
}

func (s *MemoryStore) ExpireOldOrgInvitations(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i, invitation := range s.invitations {
		if invitation.Status == "pending" && !invitation.ExpiresAt.After(now) {
			s.invitations[i].Status = "expired"
		}
	}
	return nil
}

// --- File metadata ----------------------------------------------------------

func (s *MemoryStore) InsertFile(_ context.Context, file model.File) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = append(s.files, file)
	return nil
}

func (s *MemoryStore) GetFile(_ context.Context, id string) (model.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, file := range s.files {
		if file.ID == id && file.DeletedAt == nil {
			return file, nil
		}
	}
	return model.File{}, fmt.Errorf("%w: file not found", ErrNotFound)
}

func (s *MemoryStore) ListFilesByRef(_ context.Context, refType, refID string) ([]model.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.File
	for _, file := range s.files {
		if file.RefType == refType && file.RefID == refID && file.DeletedAt == nil {
			out = append(out, file)
		}
	}
	return out, nil
}

func (s *MemoryStore) ListFilesByOwner(_ context.Context, ownerID string, limit, offset int) ([]model.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var mine []model.File
	for _, file := range s.files {
		if file.OwnerID == ownerID && file.DeletedAt == nil {
			mine = append(mine, file)
		}
	}
	if offset >= len(mine) {
		return []model.File{}, nil
	}
	end := offset + limit
	if end > len(mine) {
		end = len(mine)
	}
	return mine[offset:end], nil
}

func (s *MemoryStore) SoftDeleteFile(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, file := range s.files {
		if file.ID == id {
			now := time.Now()
			s.files[i].DeletedAt = &now
			return nil
		}
	}
	return fmt.Errorf("%w: file not found", ErrNotFound)
}

func (s *MemoryStore) ListExportFilesOlderThan(_ context.Context, olderThan time.Time) ([]model.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.File
	for _, file := range s.files {
		if file.RefType == "export" && file.DeletedAt == nil && file.CreatedAt.Before(olderThan) {
			out = append(out, file)
		}
	}
	return out, nil
}
