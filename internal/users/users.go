// Package users owns the account aggregate: the User type, its store port,
// the management service, and the admin CRUD HTTP surface. The User type is
// referenced across the stack (auth sessions, API keys, exports, search), so
// the package stays dependency-light: kernel and cache only, plus httpx on
// the HTTP side.
package users

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/your-org/go-template/internal/cache"
	"github.com/your-org/go-template/internal/kernel"
)

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

// Store persists user records for management flows.
type Store interface {
	CreateUser(ctx context.Context, user User, passwordHash string) error
	UpdateUser(ctx context.Context, user User) error
	DeleteUser(ctx context.Context, id string) error
	GetUser(ctx context.Context, id string) (User, error)
	ListUsers(ctx context.Context, opts kernel.ListOptions) ([]User, string, error)
	CountUsers(ctx context.Context) int
}

// Service implements user management on top of a store.
type Service struct {
	store Store
	// cache is optional (nil in dev mode without Redis): hot user reads are
	// served through it and invalidated on writes.
	cache  cache.Cache
	group  singleflight.Group
	logger *slog.Logger
}

// NewService builds the user management service.
func NewService(store Store) *Service { return &Service{store: store} }

// WithCache attaches the read-through cache; write paths invalidate.
func (s *Service) WithCache(c cache.Cache) *Service { s.cache = c; return s }

// WithLogger gives the cache helpers a place for decode warnings.
func (s *Service) WithLogger(logger *slog.Logger) *Service { s.logger = logger; return s }

// Create adds a user without a password credential. externalRef is the
// correlation id the enrichment streams use; empty is fine.
func (s *Service) Create(ctx context.Context, email, name, externalRef string) (User, error) {
	if !strings.Contains(email, "@") {
		return User{}, kernel.Invalid("email must be valid")
	}
	now := time.Now()
	user := User{ID: kernel.NewID(), Email: email, Name: name, ExternalRef: externalRef, CreatedAt: now, UpdatedAt: now}
	return user, s.store.CreateUser(ctx, user, "")
}

// Update rewrites the email, name, and external_ref of an existing user.
// The enrichment columns are stream-owned and never touched from here.
func (s *Service) Update(ctx context.Context, id, email, name, externalRef string) (User, error) {
	if !strings.Contains(email, "@") {
		return User{}, kernel.Invalid("email must be valid")
	}
	user, err := s.store.GetUser(ctx, id)
	if err != nil {
		return User{}, err
	}
	user.Email = email
	user.Name = name
	user.ExternalRef = externalRef
	if err := s.store.UpdateUser(ctx, user); err != nil {
		return User{}, err
	}
	s.invalidateUser(id)
	return user, nil
}

// Delete removes the user by id.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.store.DeleteUser(ctx, id); err != nil {
		return err
	}
	s.invalidateUser(id)
	return nil
}

// Get returns the user by id, served through the read-through cache when
// one is attached (the hottest read in the app: every authenticated request
// resolves its user).
func (s *Service) Get(ctx context.Context, id string) (User, error) {
	return cache.CachedGetOrLoad(ctx, s.cache, s.logger, &s.group, cache.UserCacheKey(id), cache.CacheTTLUser, func(ctx context.Context) (User, error) {
		return s.store.GetUser(ctx, id)
	})
}

func (s *Service) invalidateUser(id string) {
	if s.cache != nil {
		s.cache.Delete(context.Background(), cache.UserCacheKey(id))
	}
}

// List returns one page of users plus the next-page cursor.
func (s *Service) List(ctx context.Context, opts kernel.ListOptions) ([]User, string, error) {
	return s.store.ListUsers(ctx, opts)
}

// Count returns the total number of users.
func (s *Service) Count(ctx context.Context) int {
	return s.store.CountUsers(ctx)
}
