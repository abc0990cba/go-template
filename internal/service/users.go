package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
)

// UserService implements user management on top of a store.
type UserService struct {
	store UsersRepo
	// cache is optional (nil in dev mode without Redis): hot user reads are
	// served through it and invalidated on writes.
	cache  Cache
	group  singleflight.Group
	logger *slog.Logger
}

// WithCache attaches the read-through cache; write paths invalidate.
func (s *UserService) WithCache(cache Cache) *UserService { s.cache = cache; return s }

// WithLogger gives the cache helpers a place for decode warnings.
func (s *UserService) WithLogger(logger *slog.Logger) *UserService { s.logger = logger; return s }

// Create adds a user without a password credential. externalRef is the
// correlation id the enrichment streams use; empty is fine.
func (s *UserService) Create(ctx context.Context, email, name, externalRef string) (model.User, error) {
	if !strings.Contains(email, "@") {
		return model.User{}, validationError("email must be valid")
	}
	now := time.Now()
	user := model.User{ID: newID(), Email: email, Name: name, ExternalRef: externalRef, CreatedAt: now, UpdatedAt: now}
	return user, s.store.CreateUser(ctx, user, "")
}

// Update rewrites the email, name, and external_ref of an existing user.
// The enrichment columns are stream-owned and never touched from here.
func (s *UserService) Update(ctx context.Context, id, email, name, externalRef string) (model.User, error) {
	if !strings.Contains(email, "@") {
		return model.User{}, validationError("email must be valid")
	}
	user, err := s.store.GetUser(ctx, id)
	if err != nil {
		return model.User{}, err
	}
	user.Email = email
	user.Name = name
	user.ExternalRef = externalRef
	if err := s.store.UpdateUser(ctx, user); err != nil {
		return model.User{}, err
	}
	s.invalidateUser(id)
	return user, nil
}

// Delete removes the user by id.
func (s *UserService) Delete(ctx context.Context, id string) error {
	if err := s.store.DeleteUser(ctx, id); err != nil {
		return err
	}
	s.invalidateUser(id)
	return nil
}

// Get returns the user by id, served through the read-through cache when
// one is attached (the hottest read in the app: every authenticated request
// resolves its user).
func (s *UserService) Get(ctx context.Context, id string) (model.User, error) {
	return CachedGetOrLoad(ctx, s.cache, s.logger, &s.group, UserCacheKey(id), CacheTTLUser, func(ctx context.Context) (model.User, error) {
		return s.store.GetUser(ctx, id)
	})
}

func (s *UserService) invalidateUser(id string) {
	if s.cache != nil {
		s.cache.Delete(context.Background(), UserCacheKey(id))
	}
}

// List returns one page of users plus the next-page cursor.
func (s *UserService) List(ctx context.Context, opts repository.ListUsersOptions) ([]model.User, string, error) {
	return s.store.ListUsers(ctx, opts)
}

// Count returns the total number of users.
func (s *UserService) Count(ctx context.Context) int {
	return s.store.CountUsers(ctx)
}
