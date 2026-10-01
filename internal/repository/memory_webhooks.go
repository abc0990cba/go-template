// In-memory webhook endpoints and deliveries (the Store contract lives in
// the webhooks feature package).
package repository

import (
	"context"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/webhooks"
)

var _ webhooks.Store = (*MemoryStore)(nil)

// InsertWebhookEndpoint stores an endpoint.
func (s *MemoryStore) InsertWebhookEndpoint(ctx context.Context, endpoint webhooks.Endpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.webhooks = append(s.webhooks, endpoint)
	return nil
}

// UpdateWebhookEndpoint overwrites the endpoint.
func (s *MemoryStore) UpdateWebhookEndpoint(ctx context.Context, endpoint webhooks.Endpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.webhooks {
		if existing.ID == endpoint.ID {
			s.webhooks[i] = endpoint
			return nil
		}
	}
	return kernel.ErrNotFound
}

// DeleteWebhookEndpoint removes the endpoint.
func (s *MemoryStore) DeleteWebhookEndpoint(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.webhooks {
		if existing.ID == id {
			s.webhooks = append(s.webhooks[:i], s.webhooks[i+1:]...)
			return nil
		}
	}
	return kernel.ErrNotFound
}

// GetWebhookEndpoint returns one endpoint.
func (s *MemoryStore) GetWebhookEndpoint(ctx context.Context, id string) (webhooks.Endpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, existing := range s.webhooks {
		if existing.ID == id {
			return existing, nil
		}
	}
	return webhooks.Endpoint{}, kernel.ErrNotFound
}

// ListWebhookEndpoints returns every endpoint.
func (s *MemoryStore) ListWebhookEndpoints(ctx context.Context) ([]webhooks.Endpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]webhooks.Endpoint, len(s.webhooks))
	copy(out, s.webhooks)
	return out, nil
}

// InsertWebhookDelivery records a pending delivery.
func (s *MemoryStore) InsertWebhookDelivery(ctx context.Context, delivery webhooks.Delivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = time.Now()
	}
	s.deliveries = append(s.deliveries, delivery)
	return nil
}

// UpdateWebhookDelivery rewrites the delivery attempt data.
func (s *MemoryStore) UpdateWebhookDelivery(ctx context.Context, delivery webhooks.Delivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.deliveries {
		if existing.ID == delivery.ID {
			delivery.CreatedAt = existing.CreatedAt
			s.deliveries[i] = delivery
			return nil
		}
	}
	return kernel.ErrNotFound
}

// ListWebhookDeliveries pages the endpoint's delivery log newest first.
func (s *MemoryStore) ListWebhookDeliveries(ctx context.Context, endpointID string, limit, offset int) ([]webhooks.Delivery, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rows []webhooks.Delivery
	for _, delivery := range s.deliveries {
		if delivery.EndpointID == endpointID {
			rows = append(rows, delivery)
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
