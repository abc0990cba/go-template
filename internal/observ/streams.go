package observ

// Prometheus implementation of streams.Observer: enrichment apply outcomes,
// outbox relay health, and dead-letter depth.

import (
	"time"

	"github.com/your-org/go-template/internal/streams"
)

// StreamsMetrics adapts the stream gauges and counters to the streams
// observer port.
type StreamsMetrics struct{}

var _ streams.Observer = StreamsMetrics{}

// NewStreamsObserver returns the Prometheus-backed streams observer.
func NewStreamsObserver() StreamsMetrics { return StreamsMetrics{} }

// EnrichmentEvent counts one applied, deduped, stale, unknown-user, or
// failed event.
func (StreamsMetrics) EnrichmentEvent(source, result string) {
	EnrichmentEvents.WithLabelValues(source, result).Inc()
}

// EnrichmentApplyDuration observes one apply.
func (StreamsMetrics) EnrichmentApplyDuration(source string, d time.Duration) {
	EnrichmentApplyDuration.WithLabelValues(source).Observe(d.Seconds())
}

// OutboxRelayed counts relay outcomes.
func (StreamsMetrics) OutboxRelayed(result string, count int) {
	OutboxRelayed.WithLabelValues(result).Add(float64(count))
}

// DeadLetterParked bumps the parked counter source.
func (StreamsMetrics) DeadLetterParked() {}

// DeadLetterReplayed counts a successful replay.
func (StreamsMetrics) DeadLetterReplayed() { DeadLettersReplayed.Inc() }

// GaugesRefreshed sets the depth gauges.
func (StreamsMetrics) GaugesRefreshed(outboxPending, deadLettersPending int) {
	OutboxPending.Set(float64(outboxPending))
	DeadLettersPending.Set(float64(deadLettersPending))
}
