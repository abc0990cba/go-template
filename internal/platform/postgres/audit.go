// Audit trail persistence on PostgreSQL.
package postgres

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/platform/postgres/db"
	"github.com/your-org/go-template/internal/repository"
	"github.com/your-org/go-template/internal/service"
)

var _ service.AuditRepo = (*Store)(nil)

// RecordAudit appends one immutable record.
func (s *Store) RecordAudit(ctx context.Context, entry model.AuditEntry) error {
	return mapErr(s.q.InsertAudit(ctx, db.InsertAuditParams{
		ActorID: entry.ActorID, ActorEmail: entry.ActorEmail, Action: entry.Action,
		ResourceType: entry.ResourceType, ResourceID: entry.ResourceID, Domain: entry.Domain,
		Column7: entry.Diff, Ip: entry.IP, RequestID: entry.RequestID,
		CreatedAt: entry.CreatedAt,
	}))
}

// ListAudit pages the trail newest first with the optional filters.
func (s *Store) ListAudit(ctx context.Context, opts repository.AuditOptions) ([]model.AuditEntry, error) {
	limit := int32(25)
	if opts.Limit > 0 && opts.Limit <= 100 {
		limit = int32(opts.Limit)
	}
	off := int32(0)
	if opts.Offset > 0 {
		off = int32(min(opts.Offset, 1<<31-1)) // #nosec G115 -- clamped to int32 range
	}
	rows, err := s.q.ListAuditOffset(ctx, db.ListAuditOffsetParams{
		Actor: opts.Actor, Action: opts.Action, Resource: opts.Resource, Lim: limit, Off: off,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	entries := make([]model.AuditEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, model.AuditEntry{
			ID: strconv.FormatInt(row.ID, 10), ActorID: row.ActorID, ActorEmail: row.ActorEmail,
			Action: row.Action,
			ResourceType: row.ResourceType, ResourceID: row.ResourceID, Domain: row.Domain,
			Diff: row.Diff, IP: row.Ip, RequestID: row.RequestID, CreatedAt: row.CreatedAt,
		})
	}
	return entries, nil
}

// DeleteAuditOlderThan implements retention; wired to the cleanup cron.
func (s *Store) DeleteAuditOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	n, err := s.q.DeleteAuditOlderThan(ctx, cutoff)
	return n, mapErr(err)
}

var _ service.SettingsRepo = (*Store)(nil)

// UpsertSetting stores one setting.
func (s *Store) UpsertSetting(ctx context.Context, setting model.Setting) error {
	return mapErr(s.q.UpsertSetting(ctx, db.UpsertSettingParams{
		Key: setting.Key, Column2: setting.Value, Description: setting.Description,
		UpdatedBy: setting.UpdatedBy, UpdatedAt: setting.UpdatedAt,
	}))
}

// GetSetting returns one setting by key.
func (s *Store) GetSetting(ctx context.Context, key string) (model.Setting, error) {
	row, err := s.q.GetSetting(ctx, key)
	if err != nil {
		return model.Setting{}, mapErr(err)
	}
	return settingFromRow(row.Key, row.Value, row.Description, row.UpdatedBy, row.UpdatedAt), nil
}

// ListSettings returns every setting ordered by key.
func (s *Store) ListSettings(ctx context.Context) ([]model.Setting, error) {
	rows, err := s.q.ListSettings(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]model.Setting, 0, len(rows))
	for _, row := range rows {
		out = append(out, settingFromRow(row.Key, row.Value, row.Description, row.UpdatedBy, row.UpdatedAt))
	}
	return out, nil
}

func settingFromRow(key, value, description, updatedBy string, updatedAt time.Time) model.Setting {
	return model.Setting{Key: key, Value: value, Description: description, UpdatedBy: updatedBy, UpdatedAt: updatedAt}
}

var _ service.NotificationsRepo = (*Store)(nil)

// CreateNotification appends one notification.
func (s *Store) CreateNotification(ctx context.Context, notification model.Notification) error {
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
func (s *Store) ListNotifications(ctx context.Context, userID string, limit, offset int) ([]model.Notification, error) {
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
	out := make([]model.Notification, 0, len(rows))
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
		return repository.ErrNotFound
	}
	return nil
}

// MarkAllNotificationsRead stamps every unread notification of the user.
func (s *Store) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	_, err := s.q.MarkAllNotificationsRead(ctx, userID)
	return mapErr(err)
}

// GetNotification returns one notification by id.
func (s *Store) GetNotification(ctx context.Context, id string) (model.Notification, error) {
	row, err := s.q.GetNotification(ctx, id)
	if err != nil {
		return model.Notification{}, mapErr(err)
	}
	return notificationFromRow(row.ID, row.UserID, row.Type, row.Title, row.Body, row.Link, row.ReadAt, row.CreatedAt), nil
}

// DeleteDemoNotifications drops demo-seeded notifications (demo reset).
func (s *Store) DeleteDemoNotifications(ctx context.Context) error {
	_, err := s.q.DeleteDemoNotifications(ctx)
	return mapErr(err)
}

func notificationFromRow(id, userID, typ, title, body, link string, readAt pgtype.Timestamptz, createdAt time.Time) model.Notification {
	var read *time.Time
	if readAt.Valid {
		t := readAt.Time
		read = &t
	}
	return model.Notification{ID: id, UserID: userID, Type: typ, Title: title, Body: body, Link: link, ReadAt: read, CreatedAt: createdAt}
}

// SetPasswordCredential upserts the stored password hash; "" removes it.
func (s *Store) SetPasswordCredential(ctx context.Context, userID, passwordHash string) error {
	if passwordHash == "" {
		_, err := s.q.DeletePasswordCredential(ctx, userID)
		return mapErr(err)
	}
	return mapErr(s.q.UpsertPasswordCredential(ctx, db.UpsertPasswordCredentialParams{
		UserID: userID, PasswordHash: passwordHash,
	}))
}

var _ service.EmailTokensRepo = (*Store)(nil)

// CreateEmailToken stores a hashed single-use token.
func (s *Store) CreateEmailToken(ctx context.Context, token model.EmailToken, tokenHash string) error {
	return mapErr(s.q.CreateEmailToken(ctx, db.CreateEmailTokenParams{
		ID: token.ID, UserID: token.UserID, Kind: token.Kind,
		TokenHash: tokenHash, ExpiresAt: token.ExpiresAt, CreatedAt: token.CreatedAt,
	}))
}

// GetEmailTokenByHash resolves a token by its hash.
func (s *Store) GetEmailTokenByHash(ctx context.Context, tokenHash string) (model.EmailToken, error) {
	row, err := s.q.GetEmailTokenByHash(ctx, tokenHash)
	if err != nil {
		return model.EmailToken{}, mapErr(err)
	}
	var used *time.Time
	if row.UsedAt.Valid {
		t := row.UsedAt.Time
		used = &t
	}
	return model.EmailToken{
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
		return repository.ErrNotFound
	}
	return nil
}

var _ service.APIKeysRepo = (*Store)(nil)

// CreateAPIKey stores a key with its hash.
func (s *Store) CreateAPIKey(ctx context.Context, key model.APIKey, keyHash string) error {
	var expires pgtype.Timestamptz
	if key.ExpiresAt != nil {
		expires = pgtype.Timestamptz{Time: *key.ExpiresAt, Valid: true}
	}
	scopes := make([]string, 0, len(key.Scopes))
	scopes = append(scopes, key.Scopes...)
	return mapErr(s.q.InsertAPIKey(ctx, db.InsertAPIKeyParams{
		ID: key.ID, UserID: key.UserID, Name: key.Name, Prefix: key.Prefix,
		KeyHash: keyHash, Scopes: scopes, OrgID: key.OrgID,
		ExpiresAt: expires, CreatedAt: key.CreatedAt,
	}))
}

// ListAPIKeysForUser returns the user's keys newest first.
func (s *Store) ListAPIKeysForUser(ctx context.Context, userID string) ([]model.APIKey, error) {
	rows, err := s.q.ListAPIKeysForUser(ctx, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]model.APIKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, apiKeyFromRow(row.ID, row.UserID, row.Name, row.Prefix, row.Scopes,
			row.OrgID, row.LastUsedAt, row.ExpiresAt, row.RevokedAt, row.CreatedAt))
	}
	return out, nil
}

// GetAPIKeyByPrefix resolves a live key by prefix and returns its hash.
func (s *Store) GetAPIKeyByPrefix(ctx context.Context, prefix string) (model.APIKey, string, error) {
	row, err := s.q.GetAPIKeyByPrefix(ctx, prefix)
	if err != nil {
		return model.APIKey{}, "", mapErr(err)
	}
	key := apiKeyFromRow(row.ID, row.UserID, row.Name, row.Prefix, row.Scopes,
		row.OrgID, row.LastUsedAt, row.ExpiresAt, row.RevokedAt, row.CreatedAt)
	return key, row.KeyHash, nil
}

// TouchAPIKey stamps last use.
func (s *Store) TouchAPIKey(ctx context.Context, id string) error {
	return mapErr(s.q.TouchAPIKey(ctx, id))
}

// RevokeAPIKey marks the key revoked.
func (s *Store) RevokeAPIKey(ctx context.Context, id, userID string) error {
	n, err := s.q.RevokeAPIKey(ctx, db.RevokeAPIKeyParams{ID: id, UserID: userID})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func apiKeyFromRow(id, userID, name, prefix string, scopes []string, orgID string,
	lastUsed, expires, revoked pgtype.Timestamptz, createdAt time.Time,
) model.APIKey {
	nullable := func(t pgtype.Timestamptz) *time.Time {
		if !t.Valid {
			return nil
		}
		v := t.Time
		return &v
	}
	return model.APIKey{
		ID: id, UserID: userID, Name: name, Prefix: prefix,
		Scopes: scopes, OrgID: orgID,
		LastUsedAt: nullable(lastUsed), ExpiresAt: nullable(expires), RevokedAt: nullable(revoked),
		CreatedAt: createdAt,
	}
}

var _ service.WebhooksRepo = (*Store)(nil)

// InsertWebhookEndpoint stores an endpoint.
func (s *Store) InsertWebhookEndpoint(ctx context.Context, endpoint model.WebhookEndpoint) error {
	scopes := append([]string{}, endpoint.Events...)
	return mapErr(s.q.InsertWebhookEndpoint(ctx, db.InsertWebhookEndpointParams{
		ID: endpoint.ID, Url: endpoint.URL, Secret: endpoint.Secret,
		Events: scopes, Description: endpoint.Description, Active: endpoint.Active,
		OrgID: endpoint.OrgID, CreatedBy: endpoint.CreatedBy,
		CreatedAt: endpoint.CreatedAt, UpdatedAt: endpoint.UpdatedAt,
	}))
}

// UpdateWebhookEndpoint overwrites the endpoint.
func (s *Store) UpdateWebhookEndpoint(ctx context.Context, endpoint model.WebhookEndpoint) error {
	return mapErr(s.q.UpdateWebhookEndpoint(ctx, db.UpdateWebhookEndpointParams{
		ID: endpoint.ID, Url: endpoint.URL, Events: endpoint.Events,
		Description: endpoint.Description, Active: endpoint.Active,
	}))
}

// DeleteWebhookEndpoint removes the endpoint; deliveries cascade.
func (s *Store) DeleteWebhookEndpoint(ctx context.Context, id string) error {
	n, err := s.q.DeleteWebhookEndpoint(ctx, id)
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// GetWebhookEndpoint returns one endpoint.
func (s *Store) GetWebhookEndpoint(ctx context.Context, id string) (model.WebhookEndpoint, error) {
	row, err := s.q.GetWebhookEndpoint(ctx, id)
	if err != nil {
		return model.WebhookEndpoint{}, mapErr(err)
	}
	return webhookEndpointFromRow(row.ID, row.Url, row.Secret, row.Events, row.Description,
		row.Active, row.OrgID, row.CreatedBy, row.CreatedAt, row.UpdatedAt), nil
}

// ListWebhookEndpoints returns every endpoint.
func (s *Store) ListWebhookEndpoints(ctx context.Context) ([]model.WebhookEndpoint, error) {
	rows, err := s.q.ListWebhookEndpoints(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]model.WebhookEndpoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, webhookEndpointFromRow(row.ID, row.Url, row.Secret, row.Events, row.Description,
			row.Active, row.OrgID, row.CreatedBy, row.CreatedAt, row.UpdatedAt))
	}
	return out, nil
}

// InsertWebhookDelivery records a pending delivery.
func (s *Store) InsertWebhookDelivery(ctx context.Context, delivery model.WebhookDelivery) error {
	return mapErr(s.q.InsertWebhookDelivery(ctx, db.InsertWebhookDeliveryParams{
		ID: delivery.ID, EndpointID: delivery.EndpointID, Event: delivery.Event,
		Column4: delivery.Payload, Status: delivery.Status, CreatedAt: delivery.CreatedAt,
	}))
}

// UpdateWebhookDelivery rewrites the delivery attempt data.
func (s *Store) UpdateWebhookDelivery(ctx context.Context, delivery model.WebhookDelivery) error {
	var deliveredAt pgtype.Timestamptz
	if delivery.DeliveredAt != nil {
		deliveredAt = pgtype.Timestamptz{Time: *delivery.DeliveredAt, Valid: true}
	}
	return mapErr(s.q.UpdateWebhookDelivery(ctx, db.UpdateWebhookDeliveryParams{
		ID: delivery.ID, Status: delivery.Status, Attempts: int32(delivery.Attempts),
		LastResponseStatus: int32(delivery.LastResponseStatus), LastError: delivery.LastError,
		DeliveredAt: deliveredAt,
	}))
}

// ListWebhookDeliveries pages the endpoint's delivery log newest first.
func (s *Store) ListWebhookDeliveries(ctx context.Context, endpointID string, limit, offset int) ([]model.WebhookDelivery, error) {
	lim := int32(25)
	if limit > 0 && limit <= 100 {
		lim = int32(limit)
	}
	off := int32(0)
	if offset > 0 {
		off = int32(min(offset, 1<<31-1)) // #nosec G115 -- clamped to int32 range
	}
	rows, err := s.q.ListWebhookDeliveries(ctx, db.ListWebhookDeliveriesParams{EndpointID: endpointID, Lim: lim, Off: off})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]model.WebhookDelivery, 0, len(rows))
	for _, row := range rows {
		var delivered *time.Time
		if row.DeliveredAt.Valid {
			t := row.DeliveredAt.Time
			delivered = &t
		}
		out = append(out, model.WebhookDelivery{
			ID: row.ID, EndpointID: row.EndpointID, Event: row.Event, Payload: row.Payload,
			Status: row.Status, Attempts: int(row.Attempts), LastResponseStatus: int(row.LastResponseStatus),
			LastError: row.LastError, DeliveredAt: delivered, CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

func webhookEndpointFromRow(id, rawURL, secret string, events []string, description string,
	active bool, orgID, createdBy string, createdAt, updatedAt time.Time,
) model.WebhookEndpoint {
	return model.WebhookEndpoint{
		ID: id, URL: rawURL, Secret: secret, Events: events, Description: description,
		Active: active, OrgID: orgID, CreatedBy: createdBy, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
}
