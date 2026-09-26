package service

import (
	"context"
	"time"

	"github.com/your-org/go-template/internal/model"
)

// NotificationsService delivers in-app notifications: persistence plus a
// live publish callback wired to the realtime hub, so connected browsers
// toast and badge immediately.
type NotificationsService struct {
	store   NotificationsRepo
	publish func(model.Notification)
}

// NewNotificationsService builds the service; publish may be nil (then the
// notification is only persisted).
func NewNotificationsService(store NotificationsRepo, publish func(model.Notification)) *NotificationsService {
	return &NotificationsService{store: store, publish: publish}
}

// Create persists and broadcasts one notification. Failures degrade to an
// error for the caller; producers decide whether to care.
func (s *NotificationsService) Create(ctx context.Context, userID, typ, title, body, link string) error {
	if userID == "" || s.store == nil {
		return nil
	}
	notification := model.Notification{
		ID: newID(), UserID: userID, Type: typ, Title: title, Body: body, Link: link,
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
func (s *NotificationsService) List(ctx context.Context, userID string, limit, offset int) ([]model.Notification, error) {
	return s.store.ListNotifications(ctx, userID, limit, offset)
}

// UnreadCount returns the user's unread badge count.
func (s *NotificationsService) UnreadCount(ctx context.Context, userID string) (int64, error) {
	return s.store.CountUnreadNotifications(ctx, userID)
}

// MarkRead stamps one notification as read; the id must belong to the user.
func (s *NotificationsService) MarkRead(ctx context.Context, userID, id string) error {
	return s.store.MarkNotificationRead(ctx, id, userID)
}

// MarkAllRead stamps every unread notification of the user.
func (s *NotificationsService) MarkAllRead(ctx context.Context, userID string) error {
	return s.store.MarkAllNotificationsRead(ctx, userID)
}
