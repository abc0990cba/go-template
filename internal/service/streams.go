package service

// StreamsService consumes user-enrichment events from the Kafka and NATS
// streams, applies them exactly-once, parks poison messages in the
// dead-letter queue, and drains the transactional outbox to the brokers.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/model"
	"github.com/your-org/go-template/internal/reliability"
	"github.com/your-org/go-template/internal/repository"
)

// ErrUnknownUser marks an event whose external_ref matches no user; the
// adapters park it in the dead-letter queue instead of retrying forever.
var ErrUnknownUser = errors.New("unknown external_ref")

// ErrAlreadyReplayed marks a replay attempt on a dead letter that was
// already replayed.
var ErrAlreadyReplayed = errors.New("dead letter already replayed")

// StreamsObserver receives stream lifecycle events; the metrics stack
// implements it without coupling the domain to Prometheus. Implementations
// must be safe for concurrent use.
type StreamsObserver interface {
	EnrichmentEvent(source, result string)
	EnrichmentApplyDuration(source string, d time.Duration)
	OutboxRelayed(result string, count int)
	DeadLetterParked()
	DeadLetterReplayed()
	GaugesRefreshed(outboxPending, deadLettersPending int)
}

// noopStreamsObserver is the off switch: a nil observer in Deps means no
// metrics are recorded.
type noopStreamsObserver struct{}

func (noopStreamsObserver) EnrichmentEvent(string, string)                {}
func (noopStreamsObserver) EnrichmentApplyDuration(string, time.Duration) {}
func (noopStreamsObserver) OutboxRelayed(string, int)                     {}
func (noopStreamsObserver) DeadLetterParked()                             {}
func (noopStreamsObserver) DeadLetterReplayed()                           {}
func (noopStreamsObserver) GaugesRefreshed(int, int)                      {}

// StreamsService turns broker messages into exactly-once user updates.
type StreamsService struct {
	store     StreamsRepo
	observer  StreamsObserver
	publisher EventPublisher
	logger    *slog.Logger

	// unknownUserAttempts delays parking an event whose user has not
	// appeared yet — users are often created milliseconds after their first
	// enrichment event lands.
	unknownUserAttempts int
	unknownUserDelay    time.Duration
}

// NewStreamsService builds the service; a nil observer records nothing.
func NewStreamsService(store StreamsRepo, observer StreamsObserver, logger *slog.Logger) *StreamsService {
	if observer == nil {
		observer = noopStreamsObserver{}
	}
	return &StreamsService{
		store:               store,
		observer:            observer,
		logger:              logger,
		unknownUserAttempts: 3,
		unknownUserDelay:    200 * time.Millisecond,
	}
}

// streamEnvelope is the wire format of both enrichment streams: an identity
// envelope plus a raw JSON object of the fields the source owns.
type streamEnvelope struct {
	EventID     string          `json:"event_id"`
	ExternalRef string          `json:"external_ref"`
	Seq         int64           `json:"seq"`
	OccurredAt  time.Time       `json:"occurred_at"`
	Data        json.RawMessage `json:"data"`
}

// DecodeProfileEvent parses a Kafka user-profile event; malformed payloads
// come back wrapped in reliability.PermanentError so callers dead-letter
// instead of retrying.
func DecodeProfileEvent(payload []byte) (model.StreamEvent, error) {
	return decodeStreamEvent(model.SourceKafka, payload)
}

// DecodeContactEvent parses a NATS contact event.
func DecodeContactEvent(payload []byte) (model.StreamEvent, error) {
	return decodeStreamEvent(model.SourceNats, payload)
}

func decodeStreamEvent(source string, payload []byte) (model.StreamEvent, error) {
	var env streamEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return model.StreamEvent{}, reliability.PermanentError{Err: fmt.Errorf("decode %s event: %w", source, err)}
	}
	var problems []string
	if strings.TrimSpace(env.EventID) == "" {
		problems = append(problems, "event_id is required")
	}
	if strings.TrimSpace(env.ExternalRef) == "" {
		problems = append(problems, "external_ref is required")
	}
	if env.Seq < 0 {
		problems = append(problems, "seq must not be negative")
	}
	if len(env.Data) == 0 || !json.Valid(env.Data) || env.Data[0] != '{' {
		problems = append(problems, "data must be a JSON object")
	}
	if len(problems) > 0 {
		return model.StreamEvent{}, reliability.PermanentError{Err: fmt.Errorf("invalid %s event: %s", source, strings.Join(problems, "; "))}
	}
	occurredAt := env.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return model.StreamEvent{
		Source:      source,
		EventID:     env.EventID,
		ExternalRef: env.ExternalRef,
		Seq:         env.Seq,
		OccurredAt:  occurredAt,
		Payload:     string(env.Data),
	}, nil
}

// HandleProfileEvent applies one Kafka user-profile event. Terminal problems
// (malformed payload, unknown user) are parked in the dead-letter queue and
// returned as PermanentError / ErrUnknownUser so the adapter can ack;
// anything else is transient and safe to redeliver.
func (s *StreamsService) HandleProfileEvent(ctx context.Context, topic string, payload []byte, headers map[string]string) error {
	return s.handle(ctx, model.SourceKafka, topic, payload, headers)
}

// HandleContactEvent applies one NATS contact event.
func (s *StreamsService) HandleContactEvent(ctx context.Context, topic string, payload []byte, headers map[string]string) error {
	return s.handle(ctx, model.SourceNats, topic, payload, headers)
}

func (s *StreamsService) handle(ctx context.Context, source, topic string, payload []byte, headers map[string]string) error {
	event, err := decodeStreamEvent(source, payload)
	if err != nil {
		// Malformed: no amount of redelivery helps. Park for inspection.
		if parkErr := s.park(ctx, source, topic, payload, model.DeadLetterMalformed); parkErr != nil {
			s.logger.Error("park malformed event failed", "err", parkErr)
		}
		return err
	}
	event.Headers = headers

	start := time.Now()
	result, err := s.applyWithRetry(ctx, event)
	s.observer.EnrichmentApplyDuration(source, time.Since(start))
	switch {
	case err == nil:
		s.observer.EnrichmentEvent(source, string(result.Status))
		return nil
	case errors.Is(err, repository.ErrNotFound):
		if parkErr := s.park(ctx, source, topic, payload, model.DeadLetterUnknownUser); parkErr != nil {
			s.logger.Error("park unknown-user event failed", "err", parkErr)
		}
		s.observer.EnrichmentEvent(source, "unknown_user")
		return fmt.Errorf("%w: %q", ErrUnknownUser, event.ExternalRef)
	default:
		s.observer.EnrichmentEvent(source, "error")
		return err
	}
}

// park records a dead letter and refreshes the depth gauge.
func (s *StreamsService) park(ctx context.Context, source, topic string, payload []byte, reason string) error {
	if err := s.store.SaveDeadLetter(ctx, model.DeadLetter{
		ID: newID(), Source: source, Topic: topic,
		Payload: string(payload), Reason: reason, Attempts: 1,
	}); err != nil {
		return err
	}
	s.observer.DeadLetterParked()
	return s.RefreshGauges(ctx)
}

// applyWithRetry rides out transient store failures; an unknown external_ref
// also retries briefly before giving up, because the user row usually shows
// up within milliseconds.
func (s *StreamsService) applyWithRetry(ctx context.Context, event model.StreamEvent) (model.StreamApplyResult, error) {
	var result model.StreamApplyResult
	err := reliability.Retry(ctx, s.unknownUserAttempts, s.unknownUserDelay, func() error {
		applied, err := s.store.ApplyStreamEvent(ctx, event)
		if errors.Is(err, repository.ErrNotFound) {
			return err // retryable: user may still be created
		}
		if err != nil {
			return err // transient store failure: retry with backoff
		}
		result = applied
		return nil
	})
	return result, err
}

// ParkDeadLetter records an unprocessable message for inspection and replay.
// The handle path parks terminal failures itself; adapters only need this
// for their own (broker-specific) poison cases.
func (s *StreamsService) ParkDeadLetter(ctx context.Context, source, topic string, payload []byte, reason string) error {
	return s.park(ctx, source, topic, payload, reason)
}

// ReplayDeadLetter re-runs a parked message through its live handler path
// and stamps it replayed. Malformed letters fail again (they are still
// malformed); already-replayed letters report ErrAlreadyReplayed.
func (s *StreamsService) ReplayDeadLetter(ctx context.Context, id string) (model.DeadLetter, error) {
	letter, err := s.store.GetDeadLetter(ctx, id)
	if err != nil {
		return model.DeadLetter{}, err
	}
	if letter.ReplayedAt != nil {
		return letter, ErrAlreadyReplayed
	}
	switch letter.Source {
	case model.SourceKafka:
		err = s.HandleProfileEvent(ctx, letter.Topic, []byte(letter.Payload), nil)
	case model.SourceNats:
		err = s.HandleContactEvent(ctx, letter.Topic, []byte(letter.Payload), nil)
	case "outbox":
		err = s.replayOutboxLetter(ctx, letter)
	default:
		return letter, fmt.Errorf("dead letter source %q has no handler", letter.Source)
	}
	if err != nil {
		return letter, err
	}
	if err := s.store.MarkDeadLetterReplayed(ctx, id); err != nil {
		return letter, err
	}
	s.observer.DeadLetterReplayed()
	s.logger.Info("dead letter replayed", "id", id, "source", letter.Source, "reason", letter.Reason)
	return letter, nil
}

// replayOutboxLetter re-publishes an exhausted relay event. It needs a
// publisher: WithoutRelayPublisher must have been called at startup.
func (s *StreamsService) replayOutboxLetter(ctx context.Context, letter model.DeadLetter) error {
	if s.publisher == nil {
		return fmt.Errorf("no event publisher wired for outbox replay")
	}
	return s.publisher.PublishUserEvent(ctx, model.OutboxEvent{
		ID: newID(), EventType: letter.Topic, Payload: letter.Payload,
	})
}

// WithRelayPublisher wires the broker publisher used by the outbox relay and
// for replaying exhausted outbox letters.
func (s *StreamsService) WithRelayPublisher(p EventPublisher) *StreamsService {
	s.publisher = p
	return s
}

// RefreshGauges updates the outbox and dead-letter depth gauges.
func (s *StreamsService) RefreshGauges(ctx context.Context) error {
	pending, err := s.store.CountOutboxPending(ctx)
	if err != nil {
		return err
	}
	letters, err := s.store.CountDeadLettersPending(ctx)
	if err != nil {
		return err
	}
	s.observer.GaugesRefreshed(pending, letters)
	return nil
}

// IsTerminalStreamError reports whether a handler error means the message
// has been fully dealt with (parked in the dead-letter queue), so the
// broker adapter can ack or commit past it instead of redelivering forever.
func IsTerminalStreamError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrUnknownUser) {
		return true
	}
	var permanent reliability.PermanentError
	return errors.As(err, &permanent)
}

// EventPublisher publishes relayed domain events to the broker fabric; the
// Kafka and NATS adapters implement it.
type EventPublisher interface {
	PublishUserEvent(ctx context.Context, event model.OutboxEvent) error
}

// EnrichDemoPublisher publishes test enrichment events for one user; the
// bootstrap demo enricher implements it and the admin console's "send test
// event" buttons call it through the HTTP layer.
type EnrichDemoPublisher interface {
	PublishProfile(ctx context.Context, externalRef string, seq int64) (eventID string, err error)
	PublishContact(ctx context.Context, externalRef string, seq int64) (eventID string, err error)
}

// MultiPublisher publishes to every configured broker.
type MultiPublisher struct{ Publishers []EventPublisher }

// PublishUserEvent publishes through all publishers, joining failures.
func (m MultiPublisher) PublishUserEvent(ctx context.Context, event model.OutboxEvent) error {
	var errs []error
	for _, p := range m.Publishers {
		if err := p.PublishUserEvent(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

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
func (s *StreamsService) RelayOutboxOnce(ctx context.Context, publisher EventPublisher, cfg RelayConfig) (int, error) {
	events, err := s.store.ClaimOutboxBatch(ctx, cfg.Batch, cfg.LeaseSeconds, cfg.MaxAttempts)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}
	var published, failed []model.OutboxEvent
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
			if err := s.store.SaveDeadLetter(ctx, model.DeadLetter{
				ID: newID(), Source: "outbox", Topic: event.EventType,
				Payload: event.Payload, Reason: model.DeadLetterMaxAttempts, Attempts: event.Attempts + 1,
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
func (s *StreamsService) StreamSeqs(ctx context.Context, userID string) (profile, contact int64, err error) {
	return s.store.StreamSeqs(ctx, userID)
}

// Outbox pages relay events for the admin console.
func (s *StreamsService) Outbox(ctx context.Context, opts repository.ListOptions) ([]model.OutboxEvent, string, error) {
	return s.store.ListOutbox(ctx, opts)
}

// OutboxPending counts events awaiting publication.
func (s *StreamsService) OutboxPending(ctx context.Context) (int, error) {
	return s.store.CountOutboxPending(ctx)
}

// DeadLetters pages parked messages for the admin console.
func (s *StreamsService) DeadLetters(ctx context.Context, opts repository.DeadLetterOptions) ([]model.DeadLetter, string, error) {
	return s.store.ListDeadLetters(ctx, opts)
}

// DeadLettersPending counts messages awaiting replay.
func (s *StreamsService) DeadLettersPending(ctx context.Context) (int, error) {
	return s.store.CountDeadLettersPending(ctx)
}

func outboxIDs(events []model.OutboxEvent) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}
