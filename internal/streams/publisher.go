package streams

// The publisher ports: Publisher carries relayed domain events to the broker
// fabric, and DemoPublisher drives the admin console's test-event buttons.
// The Kafka and NATS adapters implement Publisher without the streams
// package importing either back.

import (
	"context"
	"errors"
)

// Publisher publishes relayed domain events to the broker fabric; the
// Kafka and NATS adapters implement it.
type Publisher interface {
	PublishUserEvent(ctx context.Context, event OutboxEvent) error
}

// DemoPublisher publishes test enrichment events for one user; the
// bootstrap demo enricher implements it and the admin console's "send test
// event" buttons call it through the HTTP layer.
type DemoPublisher interface {
	PublishProfile(ctx context.Context, externalRef string, seq int64) (eventID string, err error)
	PublishContact(ctx context.Context, externalRef string, seq int64) (eventID string, err error)
}

// MultiPublisher publishes to every configured broker.
type MultiPublisher struct{ Publishers []Publisher }

// PublishUserEvent publishes through all publishers, joining failures.
func (m MultiPublisher) PublishUserEvent(ctx context.Context, event OutboxEvent) error {
	var errs []error
	for _, p := range m.Publishers {
		if err := p.PublishUserEvent(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
