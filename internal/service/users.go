package service

import (
	"context"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/repository"
)

// UserService implements user management on top of a store.
type UserService struct {
	store *repository.MemoryStore
}

// Create adds a user without a password credential.
func (s *UserService) Create(ctx context.Context, email, name string) (model.User, error) {
	if !strings.Contains(email, "@") {
		return model.User{}, validationError("email must be valid")
	}
	now := time.Now()
	user := model.User{ID: newID(), Email: email, Name: name, CreatedAt: now, UpdatedAt: now}
	return user, s.store.CreateUser(ctx, user, "")
}

// Update rewrites the email and name of an existing user.
func (s *UserService) Update(ctx context.Context, id, email, name string) (model.User, error) {
	if !strings.Contains(email, "@") {
		return model.User{}, validationError("email must be valid")
	}
	user, err := s.store.GetUser(ctx, id)
	if err != nil {
		return model.User{}, err
	}
	user.Email = email
	user.Name = name
	err = s.store.UpdateUser(ctx, user)
	return user, err
}

// Delete removes the user by id.
func (s *UserService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteUser(ctx, id)
}

// Get returns the user by id.
func (s *UserService) Get(ctx context.Context, id string) (model.User, error) {
	return s.store.GetUser(ctx, id)
}

// List returns one page of users plus the next-page cursor.
func (s *UserService) List(ctx context.Context, opts repository.ListUsersOptions) ([]model.User, string, error) {
	return s.store.ListUsers(ctx, opts)
}
