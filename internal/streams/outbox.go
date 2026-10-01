package streams

// The transactional outbox relay and the admin-console query surface over
// the outbox and the dead-letter queue.

import (
	"context"

	"github.com/your-org/go-template/internal/kernel"
)

// RelayConfig tunes one relay pass.
type RelayConfig struct {
	Batch        int
	LeaseSeconds int
	MaxAttempts  int
}

// RelayOutboxOnce claims a leased batch, publishes it, and settles the
// outcomes. Events whose final attempt fails are parked as dead letters and
// marked processed; everything else is released for a later pass. It returns
// the number of events published.
func (s *Service) RelayOutboxOnce(ctx context.Context, publisher Publisher, cfg RelayConfig) (int, error) {
	events, err := s.store.ClaimOutboxBatch(ctx, cfg.Batch, cfg.LeaseSeconds, cfg.MaxAttempts)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}
	var published, failed []OutboxEvent
	for _, event := range events {
		if err := publisher.PublishUserEvent(ctx, event); err != nil {
			s.logger.Warn("outbox publish failed", "event", event.ID, "attempt", event.Attempts, "err", err)
			failed = append(failed, event)
			continue
		}
		published = append(published, event)
	}
	if len(published) > 0 {
		ids := outboxIDs(published)
		if err := s.store.MarkOutboxProcessed(ctx, ids); err != nil {
			return len(published), err
		}
		s.observer.OutboxRelayed("published", len(published))
	}
	for _, event := range failed {
		// ReleaseOutboxBatch counts this failure, so when the previous
		// failures plus this one reach MaxAttempts the event is parked
		// instead of released for another futile round.
		if event.Attempts+1 >= cfg.MaxAttempts {
			if err := s.store.SaveDeadLetter(ctx, DeadLetter{
				ID: kernel.NewID(), Source: "outbox", Topic: event.EventType,
				Payload: event.Payload, Reason: ReasonMaxAttempts, Attempts: event.Attempts + 1,
			}); err != nil {
				return len(published), err
			}
			if err := s.store.FailOutboxPermanently(ctx, []string{event.ID}); err != nil {
				return len(published), err
			}
			s.observer.OutboxRelayed("dead_letter", 1)
			continue
		}
	}
	if len(failed) > 0 {
		if err := s.store.ReleaseOutboxBatch(ctx, outboxIDs(failed)); err != nil {
			return len(published), err
		}
		s.observer.OutboxRelayed("failed", len(failed))
	}
	if err := s.RefreshGauges(ctx); err != nil {
		s.logger.Debug("gauge refresh failed", "err", err)
	}
	return len(published), nil
}

// StreamSeqs reports the applied per-source sequence numbers of a user.
func (s *Service) StreamSeqs(ctx context.Context, userID string) (profile, contact int64, err error) {
	return s.store.StreamSeqs(ctx, userID)
}

// Outbox pages relay events for the admin console.
func (s *Service) Outbox(ctx context.Context, opts kernel.ListOptions) ([]OutboxEvent, string, error) {
	return s.store.ListOutbox(ctx, opts)
}

// OutboxPending counts events awaiting publication.
func (s *Service) OutboxPending(ctx context.Context) (int, error) {
	return s.store.CountOutboxPending(ctx)
}

// DeadLetters pages parked messages for the admin console.
func (s *Service) DeadLetters(ctx context.Context, opts kernel.DeadLetterOptions) ([]DeadLetter, string, error) {
	return s.store.ListDeadLetters(ctx, opts)
}

// DeadLettersPending counts messages awaiting replay.
func (s *Service) DeadLettersPending(ctx context.Context) (int, error) {
	return s.store.CountDeadLettersPending(ctx)
}

func outboxIDs(events []OutboxEvent) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}
