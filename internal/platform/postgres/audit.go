// Audit trail persistence on PostgreSQL.

package postgres

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/your-org/go-template/internal/audit"
	"github.com/your-org/go-template/internal/email"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/notifications"
	"github.com/your-org/go-template/internal/platform/postgres/db"
	"github.com/your-org/go-template/internal/settings"
)

var _ audit.Store = (*Store)(nil)

// RecordAudit appends one immutable record.
func (s *Store) RecordAudit(ctx context.Context, entry audit.Entry) error {
	return mapErr(s.q.InsertAudit(ctx, db.InsertAuditParams{
		ActorID: entry.ActorID, ActorEmail: entry.ActorEmail, Action: entry.Action,
		ResourceType: entry.ResourceType, ResourceID: entry.ResourceID, Domain: entry.Domain,
		Column7: entry.Diff, Ip: entry.IP, RequestID: entry.RequestID,
		CreatedAt: entry.CreatedAt,
	}))
}

// ListAudit pages the trail newest first with the optional filters. A
// Subject filter switches to the per-user listing: everything the user did
// plus everything recorded about their account.
func (s *Store) ListAudit(ctx context.Context, opts kernel.AuditOptions) ([]audit.Entry, error) {
	limit := int32(25)
	if opts.Limit > 0 && opts.Limit <= 100 {
		limit = int32(opts.Limit)
	}
	off := int32(0)
	if opts.Offset > 0 {
		off = int32(min(opts.Offset, 1<<31-1)) // #nosec G115 -- clamped to int32 range
	}
	var rows []db.AuditLog
	var err error
	if opts.Subject != "" {
		rows, err = s.q.ListAuditSubject(ctx, db.ListAuditSubjectParams{Subject: opts.Subject, Lim: limit, Off: off})
	} else {
		rows, err = s.q.ListAuditOffset(ctx, db.ListAuditOffsetParams{
			Actor: opts.Actor, Action: opts.Action, Resource: opts.Resource,
			ResourceID: opts.ResourceID, Lim: limit, Off: off,
		})
	}
	if err != nil {
		return nil, mapErr(err)
	}
	entries := make([]audit.Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, audit.Entry{
			ID: strconv.FormatInt(row.ID, 10), ActorID: row.ActorID, ActorEmail: row.ActorEmail,
			Action:       row.Action,
			ResourceType: row.ResourceType, ResourceID: row.ResourceID, Domain: row.Domain,
			Diff: row.Diff, IP: row.Ip, RequestID: row.RequestID, CreatedAt: row.CreatedAt,
		})
	}
	return entries, nil
}

// PruneAuditLog implements retention through the audit_prune SECURITY
// DEFINER function — direct DELETE on audit_log is trigger-blocked for this
// role (migration 000022). The worker's sweep calls it on a schedule.
func (s *Store) PruneAuditLog(ctx context.Context, cutoff time.Time) (int64, error) {
	pruned, err := s.q.PruneAuditLog(ctx, cutoff)
	if err != nil {
		return 0, mapErr(err)
	}
	return pruned, nil
}

var _ settings.Store = (*Store)(nil)

// UpsertSetting stores one setting.
func (s *Store) UpsertSetting(ctx context.Context, setting settings.Setting) error {
	return mapErr(s.q.UpsertSetting(ctx, db.UpsertSettingParams{
		Key: setting.Key, Column2: setting.Value, Description: setting.Description,
		UpdatedBy: setting.UpdatedBy, UpdatedAt: setting.UpdatedAt,
	}))
}

// GetSetting returns one setting by key.
func (s *Store) GetSetting(ctx context.Context, key string) (settings.Setting, error) {
	row, err := s.q.GetSetting(ctx, key)
	if err != nil {
		return settings.Setting{}, mapErr(err)
	}
	return settingFromRow(row.Key, row.Value, row.Description, row.UpdatedBy, row.UpdatedAt), nil
}

// ListSettings returns every setting ordered by key.
func (s *Store) ListSettings(ctx context.Context) ([]settings.Setting, error) {
	rows, err := s.q.ListSettings(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]settings.Setting, 0, len(rows))
	for _, row := range rows {
		out = append(out, settingFromRow(row.Key, row.Value, row.Description, row.UpdatedBy, row.UpdatedAt))
	}
	return out, nil
}

func settingFromRow(key, value, description, updatedBy string, updatedAt time.Time) settings.Setting {
	return settings.Setting{Key: key, Value: value, Description: description, UpdatedBy: updatedBy, UpdatedAt: updatedAt}
}

var _ notifications.Store = (*Store)(nil)

// CreateNotification appends one notification.
func (s *Store) CreateNotification(ctx context.Context, notification notifications.Notification) error {
	var readAt pgtype.Timestamptz
	if notification.ReadAt != nil {
		readAt = pgtype.Timestamptz{Time: *notification.ReadAt, Valid: true}
	}
	return mapErr(s.q.InsertNotification(ctx, db.InsertNotificationParams{
		ID: notification.ID, UserID: notification.UserID, Type: notification.Type,
		Title: notification.Title, Body: notification.Body, Link: notification.Link,
		ReadAt: readAt, CreatedAt: notification.CreatedAt,
	}))
}

// ListNotifications returns the user's notifications newest first.
func (s *Store) ListNotifications(ctx context.Context, userID string, limit, offset int) ([]notifications.Notification, error) {
	lim := int32(25)
	if limit > 0 && limit <= 100 {
		lim = int32(limit)
	}
	off := int32(0)
	if offset > 0 {
		off = int32(min(offset, 1<<31-1)) // #nosec G115 -- clamped to int32 range
	}
	rows, err := s.q.ListNotifications(ctx, db.ListNotificationsParams{UserID: userID, Lim: lim, Off: off})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]notifications.Notification, 0, len(rows))
	for _, row := range rows {
		out = append(out, notificationFromRow(row.ID, row.UserID, row.Type, row.Title, row.Body, row.Link, row.ReadAt, row.CreatedAt))
	}
	return out, nil
}

// CountUnreadNotifications returns the user's unread count.
func (s *Store) CountUnreadNotifications(ctx context.Context, userID string) (int64, error) {
	return s.q.CountUnreadNotifications(ctx, userID)
}

// MarkNotificationRead stamps one notification as read.
func (s *Store) MarkNotificationRead(ctx context.Context, id, userID string) error {
	n, err := s.q.MarkNotificationRead(ctx, db.MarkNotificationReadParams{ID: id, UserID: userID})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return kernel.ErrNotFound
	}
	return nil
}

// MarkAllNotificationsRead stamps every unread notification of the user.
func (s *Store) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	_, err := s.q.MarkAllNotificationsRead(ctx, userID)
	return mapErr(err)
}

// GetNotification returns one notification by id.
func (s *Store) GetNotification(ctx context.Context, id string) (notifications.Notification, error) {
	row, err := s.q.GetNotification(ctx, id)
	if err != nil {
		return notifications.Notification{}, mapErr(err)
	}
	return notificationFromRow(row.ID, row.UserID, row.Type, row.Title, row.Body, row.Link, row.ReadAt, row.CreatedAt), nil
}

// DeleteDemoNotifications drops demo-seeded notifications (demo reset).
func (s *Store) DeleteDemoNotifications(ctx context.Context) error {
	_, err := s.q.DeleteDemoNotifications(ctx)
	return mapErr(err)
}

func notificationFromRow(id, userID, typ, title, body, link string, readAt pgtype.Timestamptz, createdAt time.Time) notifications.Notification {
	var read *time.Time
	if readAt.Valid {
		t := readAt.Time
		read = &t
	}
	return notifications.Notification{ID: id, UserID: userID, Type: typ, Title: title, Body: body, Link: link, ReadAt: read, CreatedAt: createdAt}
}

// SetPasswordCredential upserts the stored password hash; "" removes it.
func (s *Store) SetPasswordCredential(ctx context.Context, userID, passwordHash string) error {
	if passwordHash == "" {
		_, err := s.q.DeletePasswordCredential(ctx, userID)
		if err == nil {
			// A credential change proves control of the account; the
			// lockout counter starts fresh.
			_, _ = s.q.ResetLoginGuard(ctx, userID)
		}
		return mapErr(err)
	}
	if err := mapErr(s.q.UpsertPasswordCredential(ctx, db.UpsertPasswordCredentialParams{
		UserID: userID, PasswordHash: passwordHash,
	})); err != nil {
		return err
	}
	_, _ = s.q.ResetLoginGuard(ctx, userID)
	return nil
}

var _ email.Store = (*Store)(nil)

// CreateEmailToken stores a hashed single-use token.
func (s *Store) CreateEmailToken(ctx context.Context, token email.Token, tokenHash string) error {
	return mapErr(s.q.CreateEmailToken(ctx, db.CreateEmailTokenParams{
		ID: token.ID, UserID: token.UserID, Kind: token.Kind,
		TokenHash: tokenHash, ExpiresAt: token.ExpiresAt, CreatedAt: token.CreatedAt,
	}))
}

// GetEmailTokenByHash resolves a token by its hash.
func (s *Store) GetEmailTokenByHash(ctx context.Context, tokenHash string) (email.Token, error) {
	row, err := s.q.GetEmailTokenByHash(ctx, tokenHash)
	if err != nil {
		return email.Token{}, mapErr(err)
	}
	var used *time.Time
	if row.UsedAt.Valid {
		t := row.UsedAt.Time
		used = &t
	}
	return email.Token{
		ID: row.ID, UserID: row.UserID, Kind: row.Kind,
		ExpiresAt: row.ExpiresAt, UsedAt: used, CreatedAt: row.CreatedAt,
	}, nil
}

// MarkEmailTokenUsed consumes the token.
func (s *Store) MarkEmailTokenUsed(ctx context.Context, id string) error {
	n, err := s.q.MarkEmailTokenUsed(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return kernel.ErrNotFound
	}
	return nil
}
