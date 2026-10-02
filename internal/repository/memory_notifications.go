package repository

import (
	"context"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/notifications"
)

// CreateNotification appends one notification.
func (s *MemoryStore) CreateNotification(_ context.Context, notification notifications.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now()
	}
	s.notifs = append(s.notifs, notification)
	return nil
}

// ListNotifications returns the user's notifications newest first.
func (s *MemoryStore) ListNotifications(_ context.Context, userID string, limit, offset int) ([]notifications.Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rows []notifications.Notification
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
func (s *MemoryStore) CountUnreadNotifications(_ context.Context, userID string) (int64, error) {
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
func (s *MemoryStore) MarkNotificationRead(_ context.Context, id, userID string) error {
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
	return kernel.ErrNotFound
}

// MarkAllNotificationsRead stamps every unread notification of the user.
func (s *MemoryStore) MarkAllNotificationsRead(_ context.Context, userID string) error {
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
func (s *MemoryStore) GetNotification(_ context.Context, id string) (notifications.Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, notification := range s.notifs {
		if notification.ID == id {
			return notification, nil
		}
	}
	return notifications.Notification{}, kernel.ErrNotFound
}

// DeleteDemoNotifications drops demo-seeded notifications (demo reset).
func (s *MemoryStore) DeleteDemoNotifications(_ context.Context) error {
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
