// Package repository provides storage adapters for the domain models. The
// memory implementation is the development fake; a PostgreSQL adapter lives
// behind the same method surface.
package repository

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/your-org/go-template/internal/model"
)

// ErrNotFound is returned when a requested entity does not exist. Handlers
// map it to HTTP 404.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when an insert would violate a uniqueness
// constraint, such as a duplicate email or an identity owned by another user.
// Handlers map it to HTTP 409.
var ErrConflict = errors.New("conflict")

// ListUsersOptions controls user listing. Mode selects the pagination
// strategy: "offset" pages by Offset, "cursor" and "keyset" continue after
// the user id in After.
type ListUsersOptions struct {
	Query  string
	Mode   string
	Limit  int
	Offset int
	After  string
}

// MemoryStore is an in-process, mutex-guarded store. Data does not survive
// restarts, and separate processes never see each other's state.
type MemoryStore struct {
	mu          sync.RWMutex
	users       map[string]model.User
	passwords   map[string]string
	identities  []model.AuthIdentity
	sessions    map[string]string
	oauthStates map[string]model.OAuthState
	jobs        map[string]model.Job
	reports     map[string]model.Report
	enrichments []model.EnrichmentProfile
	events      []string
}

// NewMemoryStore returns an empty store ready for use.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users:       map[string]model.User{},
		passwords:   map[string]string{},
		sessions:    map[string]string{},
		oauthStates: map[string]model.OAuthState{},
		jobs:        map[string]model.Job{},
		reports:     map[string]model.Report{},
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

// UpdateUser overwrites an existing user and refreshes UpdatedAt.
func (s *MemoryStore) UpdateUser(ctx context.Context, user model.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[user.ID]; !ok {
		return ErrNotFound
	}
	user.UpdatedAt = time.Now()
	s.users[user.ID] = user
	return nil
}

// DeleteUser removes the user by id.
func (s *MemoryStore) DeleteUser(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		return ErrNotFound
	}
	delete(s.users, id)
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

// CreateSession maps an opaque session token to a user id.
func (s *MemoryStore) CreateSession(ctx context.Context, token, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = userID
	return nil
}

// GetSession returns the user id behind a session token.
func (s *MemoryStore) GetSession(ctx context.Context, token string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	userID, ok := s.sessions[token]
	if !ok {
		return "", ErrNotFound
	}
	return userID, nil
}

// DeleteSession revokes a session token.
func (s *MemoryStore) DeleteSession(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
	return nil
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

// ListJobs returns all jobs newest first.
func (s *MemoryStore) ListJobs(ctx context.Context) ([]model.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]model.Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].CreatedAt.After(jobs[j].CreatedAt) })
	return jobs, nil
}

// SaveReport upserts a report by id.
func (s *MemoryStore) SaveReport(ctx context.Context, report model.Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[report.ID] = report
	return nil
}

// ListReports returns all reports.
func (s *MemoryStore) ListReports(ctx context.Context) ([]model.Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	reports := make([]model.Report, 0, len(s.reports))
	for _, report := range s.reports {
		reports = append(reports, report)
	}
	return reports, nil
}

// AddEnrichment appends an enrichment profile record.
func (s *MemoryStore) AddEnrichment(ctx context.Context, profile model.EnrichmentProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enrichments = append(s.enrichments, profile)
	return nil
}

// AddEvent records a named analytics event.
func (s *MemoryStore) AddEvent(ctx context.Context, event string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
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
	return summary
}
