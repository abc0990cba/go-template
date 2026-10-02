package postgres

import (
	"context"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/your-org/go-template/internal/apikeys"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/platform/postgres/db"
	"github.com/your-org/go-template/internal/webhooks"
)

var _ apikeys.Store = (*Store)(nil)

// CreateAPIKey stores a key with its hash.
func (s *Store) CreateAPIKey(ctx context.Context, key apikeys.Key, keyHash string) error {
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
func (s *Store) ListAPIKeysForUser(ctx context.Context, userID string) ([]apikeys.Key, error) {
	rows, err := s.q.ListAPIKeysForUser(ctx, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]apikeys.Key, 0, len(rows))
	for _, row := range rows {
		out = append(out, apiKeyFromRow(row.ID, row.UserID, row.Name, row.Prefix, row.Scopes,
			row.OrgID, row.LastUsedAt, row.ExpiresAt, row.RevokedAt, row.CreatedAt))
	}
	return out, nil
}

// GetAPIKeyByPrefix resolves a live key by prefix and returns its hash.
func (s *Store) GetAPIKeyByPrefix(ctx context.Context, prefix string) (apikeys.Key, string, error) {
	row, err := s.q.GetAPIKeyByPrefix(ctx, prefix)
	if err != nil {
		return apikeys.Key{}, "", mapErr(err)
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
		return kernel.ErrNotFound
	}
	return nil
}

func apiKeyFromRow(id, userID, name, prefix string, scopes []string, orgID string,
	lastUsed, expires, revoked pgtype.Timestamptz, createdAt time.Time,
) apikeys.Key {
	nullable := func(t pgtype.Timestamptz) *time.Time {
		if !t.Valid {
			return nil
		}
		v := t.Time
		return &v
	}
	return apikeys.Key{
		ID: id, UserID: userID, Name: name, Prefix: prefix,
		Scopes: scopes, OrgID: orgID,
		LastUsedAt: nullable(lastUsed), ExpiresAt: nullable(expires), RevokedAt: nullable(revoked),
		CreatedAt: createdAt,
	}
}

var _ webhooks.Store = (*Store)(nil)

// InsertWebhookEndpoint stores an endpoint.
func (s *Store) InsertWebhookEndpoint(ctx context.Context, endpoint webhooks.Endpoint) error {
	scopes := append([]string{}, endpoint.Events...)
	return mapErr(s.q.InsertWebhookEndpoint(ctx, db.InsertWebhookEndpointParams{
		ID: endpoint.ID, Url: endpoint.URL, Secret: endpoint.Secret,
		Events: scopes, Description: endpoint.Description, Active: endpoint.Active,
		OrgID: endpoint.OrgID, CreatedBy: endpoint.CreatedBy,
		CreatedAt: endpoint.CreatedAt, UpdatedAt: endpoint.UpdatedAt,
	}))
}

// UpdateWebhookEndpoint overwrites the endpoint.
func (s *Store) UpdateWebhookEndpoint(ctx context.Context, endpoint webhooks.Endpoint) error {
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
		return kernel.ErrNotFound
	}
	return nil
}

// GetWebhookEndpoint returns one endpoint.
func (s *Store) GetWebhookEndpoint(ctx context.Context, id string) (webhooks.Endpoint, error) {
	row, err := s.q.GetWebhookEndpoint(ctx, id)
	if err != nil {
		return webhooks.Endpoint{}, mapErr(err)
	}
	return webhookEndpointFromRow(row.ID, row.Url, row.Secret, row.Events, row.Description,
		row.Active, row.OrgID, row.CreatedBy, row.CreatedAt, row.UpdatedAt), nil
}

// ListWebhookEndpoints returns every endpoint.
func (s *Store) ListWebhookEndpoints(ctx context.Context) ([]webhooks.Endpoint, error) {
	rows, err := s.q.ListWebhookEndpoints(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]webhooks.Endpoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, webhookEndpointFromRow(row.ID, row.Url, row.Secret, row.Events, row.Description,
			row.Active, row.OrgID, row.CreatedBy, row.CreatedAt, row.UpdatedAt))
	}
	return out, nil
}

// InsertWebhookDelivery records a pending delivery.
func (s *Store) InsertWebhookDelivery(ctx context.Context, delivery webhooks.Delivery) error {
	return mapErr(s.q.InsertWebhookDelivery(ctx, db.InsertWebhookDeliveryParams{
		ID: delivery.ID, EndpointID: delivery.EndpointID, Event: delivery.Event,
		Column4: delivery.Payload, Status: delivery.Status, CreatedAt: delivery.CreatedAt,
	}))
}

// UpdateWebhookDelivery rewrites the delivery attempt data.
func (s *Store) UpdateWebhookDelivery(ctx context.Context, delivery webhooks.Delivery) error {
	var deliveredAt pgtype.Timestamptz
	if delivery.DeliveredAt != nil {
		deliveredAt = pgtype.Timestamptz{Time: *delivery.DeliveredAt, Valid: true}
	}
	return mapErr(s.q.UpdateWebhookDelivery(ctx, db.UpdateWebhookDeliveryParams{
		ID: delivery.ID, Status: delivery.Status,
		Attempts:           int32(min(delivery.Attempts, math.MaxInt32)),           // #nosec G115 -- clamped to int32 range
		LastResponseStatus: int32(min(delivery.LastResponseStatus, math.MaxInt32)), // #nosec G115 -- clamped to int32 range
		LastError:          delivery.LastError,
		DeliveredAt:        deliveredAt,
	}))
}

// ListWebhookDeliveries pages the endpoint's delivery log newest first.
func (s *Store) ListWebhookDeliveries(ctx context.Context, endpointID string, limit, offset int) ([]webhooks.Delivery, error) {
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
	out := make([]webhooks.Delivery, 0, len(rows))
	for _, row := range rows {
		var delivered *time.Time
		if row.DeliveredAt.Valid {
			t := row.DeliveredAt.Time
			delivered = &t
		}
		out = append(out, webhooks.Delivery{
			ID: row.ID, EndpointID: row.EndpointID, Event: row.Event, Payload: row.Payload,
			Status: row.Status, Attempts: int(row.Attempts), LastResponseStatus: int(row.LastResponseStatus),
			LastError: row.LastError, DeliveredAt: delivered, CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

func webhookEndpointFromRow(id, rawURL, secret string, events []string, description string,
	active bool, orgID, createdBy string, createdAt, updatedAt time.Time,
) webhooks.Endpoint {
	return webhooks.Endpoint{
		ID: id, URL: rawURL, Secret: secret, Events: events, Description: description,
		Active: active, OrgID: orgID, CreatedBy: createdBy, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
}
