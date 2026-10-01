// Package notifications delivers in-app notifications: persistence plus a
// live publish callback wired to the realtime hub, so connected browsers
// toast and badge immediately.
package notifications

import (
	"context"
	"time"

	"github.com/your-org/go-template/internal/kernel"
)

// Notification is one in-app inbox entry for a user.
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

// Store persists per-user notifications.
type Store interface {
	CreateNotification(ctx context.Context, notification Notification) error
	ListNotifications(ctx context.Context, userID string, limit, offset int) ([]Notification, error)
	CountUnreadNotifications(ctx context.Context, userID string) (int64, error)
	MarkNotificationRead(ctx context.Context, id, userID string) error
	MarkAllNotificationsRead(ctx context.Context, userID string) error
}

// Publisher broadcasts a persisted notification to connected browsers;
// wired to the realtime hub by the assembly.
type Publisher func(Notification)

// Service creates and lists notifications.
type Service struct {
	store   Store
	publish Publisher
}

// NewService builds the service; publish may be nil (then the notification
// is only persisted).
func NewService(store Store, publish Publisher) *Service {
	return &Service{store: store, publish: publish}
}

// Create persists and broadcasts one notification. Failures degrade to an
// error for the caller; producers decide whether to care.
func (s *Service) Create(ctx context.Context, userID, typ, title, body, link string) error {
	if userID == "" || s.store == nil {
		return nil
	}
	notification := Notification{
		ID: kernel.NewID(), UserID: userID, Type: typ, Title: title, Body: body, Link: link,
		CreatedAt: time.Now(),
	}
	if err := s.store.CreateNotification(ctx, notification); err != nil {
		return err
	}
	if s.publish != nil {
		s.publish(notification)
	}
	return nil
}

// List returns one page of the user's notifications newest first.
func (s *Service) List(ctx context.Context, userID string, limit, offset int) ([]Notification, error) {
	return s.store.ListNotifications(ctx, userID, limit, offset)
}

// UnreadCount returns the user's unread badge count.
func (s *Service) UnreadCount(ctx context.Context, userID string) (int64, error) {
	return s.store.CountUnreadNotifications(ctx, userID)
}

// MarkRead stamps one notification as read; the id must belong to the user.
func (s *Service) MarkRead(ctx context.Context, userID, id string) error {
	return s.store.MarkNotificationRead(ctx, id, userID)
}

// MarkAllRead stamps every unread notification of the user.
func (s *Service) MarkAllRead(ctx context.Context, userID string) error {
	return s.store.MarkAllNotificationsRead(ctx, userID)
}
