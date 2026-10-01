// Package streams consumes user-enrichment events from the Kafka and NATS
// streams, applies them exactly-once, parks poison messages in the
// dead-letter queue, and drains the transactional outbox to the brokers.
package streams

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/reliability"
)

// ErrUnknownUser marks an event whose external_ref matches no user; the
// adapters park it in the dead-letter queue instead of retrying forever.
var ErrUnknownUser = errors.New("unknown external_ref")

// ErrAlreadyReplayed marks a replay attempt on a dead letter that was
// already replayed.
var ErrAlreadyReplayed = errors.New("dead letter already replayed")

// Observer receives stream lifecycle events; the metrics stack
// implements it without coupling the domain to Prometheus. Implementations
// must be safe for concurrent use.
type Observer interface {
	EnrichmentEvent(source, result string)
	EnrichmentApplyDuration(source string, d time.Duration)
	OutboxRelayed(result string, count int)
	DeadLetterParked()
	DeadLetterReplayed()
	GaugesRefreshed(outboxPending, deadLettersPending int)
}

// noopObserver is the off switch: a nil observer in Deps means no
// metrics are recorded.
type noopObserver struct{}

func (noopObserver) EnrichmentEvent(string, string)                {}
func (noopObserver) EnrichmentApplyDuration(string, time.Duration) {}
func (noopObserver) OutboxRelayed(string, int)                     {}
func (noopObserver) DeadLetterParked()                             {}
func (noopObserver) DeadLetterReplayed()                           {}
func (noopObserver) GaugesRefreshed(int, int)                      {}

// Service turns broker messages into exactly-once user updates.
type Service struct {
	store     Store
	observer  Observer
	publisher Publisher
	logger    *slog.Logger

	// unknownUserAttempts delays parking an event whose user has not
	// appeared yet — users are often created milliseconds after their first
	// enrichment event lands.
	unknownUserAttempts int
	unknownUserDelay    time.Duration
}

// NewService builds the service; a nil observer records nothing.
func NewService(store Store, observer Observer, logger *slog.Logger) *Service {
	if observer == nil {
		observer = noopObserver{}
	}
	return &Service{
		store:               store,
		observer:            observer,
		logger:              logger,
		unknownUserAttempts: 3,
		unknownUserDelay:    200 * time.Millisecond,
	}
}

// WithUnknownUserRetry tunes how long an event whose user has not appeared
// yet keeps retrying before it is parked (defaults: three attempts, 200ms
// apart). The pruning tests use it to shorten or disable the wait.
func (s *Service) WithUnknownUserRetry(attempts int, delay time.Duration) *Service {
	s.unknownUserAttempts = attempts
	s.unknownUserDelay = delay
	return s
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
func DecodeProfileEvent(payload []byte) (Event, error) {
	return decodeStreamEvent(SourceKafka, payload)
}

// DecodeContactEvent parses a NATS contact event.
func DecodeContactEvent(payload []byte) (Event, error) {
	return decodeStreamEvent(SourceNats, payload)
}

func decodeStreamEvent(source string, payload []byte) (Event, error) {
	var env streamEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return Event{}, reliability.PermanentError{Err: fmt.Errorf("decode %s event: %w", source, err)}
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
		return Event{}, reliability.PermanentError{Err: fmt.Errorf("invalid %s event: %s", source, strings.Join(problems, "; "))}
	}
	occurredAt := env.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return Event{
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
func (s *Service) HandleProfileEvent(ctx context.Context, topic string, payload []byte, headers map[string]string) error {
	return s.handle(ctx, SourceKafka, topic, payload, headers)
}

// HandleContactEvent applies one NATS contact event.
func (s *Service) HandleContactEvent(ctx context.Context, topic string, payload []byte, headers map[string]string) error {
	return s.handle(ctx, SourceNats, topic, payload, headers)
}

func (s *Service) handle(ctx context.Context, source, topic string, payload []byte, headers map[string]string) error {
	event, err := decodeStreamEvent(source, payload)
	if err != nil {
		// Malformed: no amount of redelivery helps. Park for inspection.
		if parkErr := s.park(ctx, source, topic, payload, ReasonMalformed); parkErr != nil {
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
	case errors.Is(err, kernel.ErrNotFound):
		if parkErr := s.park(ctx, source, topic, payload, ReasonUnknownUser); parkErr != nil {
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
func (s *Service) park(ctx context.Context, source, topic string, payload []byte, reason string) error {
	if err := s.store.SaveDeadLetter(ctx, DeadLetter{
		ID: kernel.NewID(), Source: source, Topic: topic,
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
func (s *Service) applyWithRetry(ctx context.Context, event Event) (ApplyResult, error) {
	var result ApplyResult
	err := reliability.Retry(ctx, s.unknownUserAttempts, s.unknownUserDelay, func() error {
		applied, err := s.store.ApplyStreamEvent(ctx, event)
		if errors.Is(err, kernel.ErrNotFound) {
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
func (s *Service) ParkDeadLetter(ctx context.Context, source, topic string, payload []byte, reason string) error {
	return s.park(ctx, source, topic, payload, reason)
}

// ReplayDeadLetter re-runs a parked message through its live handler path
// and stamps it replayed. Malformed letters fail again (they are still
// malformed); already-replayed letters report ErrAlreadyReplayed.
func (s *Service) ReplayDeadLetter(ctx context.Context, id string) (DeadLetter, error) {
	letter, err := s.store.GetDeadLetter(ctx, id)
	if err != nil {
		return DeadLetter{}, err
	}
	if letter.ReplayedAt != nil {
		return letter, ErrAlreadyReplayed
	}
	switch letter.Source {
	case SourceKafka:
		err = s.HandleProfileEvent(ctx, letter.Topic, []byte(letter.Payload), nil)
	case SourceNats:
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
func (s *Service) replayOutboxLetter(ctx context.Context, letter DeadLetter) error {
	if s.publisher == nil {
		return fmt.Errorf("no event publisher wired for outbox replay")
	}
	return s.publisher.PublishUserEvent(ctx, OutboxEvent{
		ID: kernel.NewID(), EventType: letter.Topic, Payload: letter.Payload,
	})
}

// WithRelayPublisher wires the broker publisher used by the outbox relay and
// for replaying exhausted outbox letters.
func (s *Service) WithRelayPublisher(p Publisher) *Service {
	s.publisher = p
	return s
}

// RefreshGauges updates the outbox and dead-letter depth gauges.
func (s *Service) RefreshGauges(ctx context.Context) error {
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

// IsTerminal reports whether a handler error means the message has been
// fully dealt with (parked in the dead-letter queue), so the broker adapter
// can ack or commit past it instead of redelivering forever.
func IsTerminal(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrUnknownUser) {
		return true
	}
	var permanent reliability.PermanentError
	return errors.As(err, &permanent)
}
