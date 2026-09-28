// Package observ wires the observability stack: OpenTelemetry tracing to an
// OTLP endpoint, Prometheus business counters, and build metadata. Tracing
// is off until APP_OTLP_ENDPOINT is set; metrics are always available.
package observ

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/your-org/go-template/internal/service"
)

// Build carries version metadata exposed on /version and stamped on the
// tracing resource. Set via ldflags at build time.
type Build struct {
	Version string
	Commit  string
	Date    string
}

var (
	tracerProvider *sdktrace.TracerProvider

	// Tracer is the application-wide tracer for hand-made spans.
	Tracer = otel.Tracer("github.com/your-org/go-template")

	// Propagator moves trace context across process boundaries (HTTP
	// headers, asynq task headers).
	Propagator = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
)

// JobsQueued counts jobs accepted by the producer.
var JobsQueued = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "app_jobs_queued_total",
	Help: "Jobs enqueued (or accepted inline) by type.",
})

// JobsProcessed counts finished jobs by final status (done, failed).
var JobsProcessed = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "app_jobs_processed_total",
	Help: "Jobs finished by final status.",
}, []string{"status"})

// JobDuration observes job execution time by type.
var JobDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "app_job_duration_seconds",
	Help:    "Job execution duration.",
	Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 15, 60, 300},
}, []string{"type"})

// CBCurrentState gauges each named circuit breaker: 0 closed, 1 half-open,
// 2 open.
var CBCurrentState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "app_cb_state",
	Help: "Circuit breaker state (0 closed, 1 half-open, 2 open).",
}, []string{"name"})

// CBTransitions counts breaker state transitions.
var CBTransitions = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "app_cb_transitions_total",
	Help: "Circuit breaker state transitions.",
}, []string{"name", "from", "to"})

// EnrichmentEvents counts stream events by source and outcome.
var EnrichmentEvents = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "app_enrichment_events_total",
	Help: "User enrichment events by source and result.",
}, []string{"source", "result"})

// EnrichmentApplyDuration observes how long applying one event takes.
var EnrichmentApplyDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "app_enrichment_apply_seconds",
	Help:    "Time to apply one enrichment event.",
	Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5},
}, []string{"source"})

// OutboxPending gauges events waiting for the relay.
var OutboxPending = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "app_outbox_pending",
	Help: "Outbox events not yet published.",
})

// OutboxRelayed counts relay outcomes by result.
var OutboxRelayed = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "app_outbox_relay_total",
	Help: "Outbox relay outcomes.",
}, []string{"result"})

// DeadLettersPending gauges parked messages awaiting replay.
var DeadLettersPending = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "app_dead_letters_pending",
	Help: "Dead letters not yet replayed.",
})

// DeadLettersReplayed counts dead letters successfully re-processed.
var DeadLettersReplayed = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "app_dead_letters_replayed_total",
	Help: "Dead letters replayed.",
})

// RateLimitRejected counts requests shed by rate limiters.
var RateLimitRejected = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "app_rate_limit_rejected_total",
	Help: "Requests rejected by rate limiting.",
}, []string{"scope"})

// IdempotencyReplays counts replayed idempotent responses.
var IdempotencyReplays = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "app_idempotency_replays_total",
	Help: "Responses replayed from the idempotency store.",
})

// RateLimitErrors counts limiter infrastructure failures (fail-open events).
var RateLimitErrors = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "app_rate_limit_errors_total",
	Help: "Rate limiter infrastructure failures; requests fail open.",
})

func init() {
	prometheus.MustRegister(JobsQueued, JobsProcessed, JobDuration,
		CBCurrentState, CBTransitions, EnrichmentEvents, EnrichmentApplyDuration,
		OutboxPending, OutboxRelayed, DeadLettersPending, DeadLettersReplayed,
		RateLimitRejected, RateLimitErrors, IdempotencyReplays)
}

// RecordBreakerState applies one transition to the gauges; adapters call it
// from Breaker.OnStateChange with reliability.CircuitState values.
func RecordBreakerState(name string, from, to int) {
	CBCurrentState.WithLabelValues(name).Set(float64(to))
	CBTransitions.WithLabelValues(name, stateName(from), stateName(to)).Inc()
}

func stateName(state int) string {
	switch state {
	case 2:
		return "open"
	case 1:
		return "half-open"
	default:
		return "closed"
	}
}

// SetupTracing installs the global tracer provider when endpoint is
// non-empty and returns a shutdown function; otherwise it returns a no-op.
func SetupTracing(ctx context.Context, endpoint string, build Build) (func(context.Context) error, error) {
	if endpoint == "" {
		otel.SetTracerProvider(noopTracerProvider())
		otel.SetTextMapPropagator(Propagator)
		return func(context.Context) error { return nil }, nil
	}
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("go-template"),
			semconv.ServiceVersion(build.Version),
		)),
	)
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(Propagator)
	return tracerProvider.Shutdown, nil
}

// TracingEnabled reports whether spans should be created and propagated.
func TracingEnabled() bool { return tracerProvider != nil }

// noopTracerProvider keeps the global provider valid (and span creation
// cheap) when tracing is disabled.
func noopTracerProvider() trace.TracerProvider {
	return noop.NewTracerProvider()
}

// SpanFromHeaders restores a remote span context from a string map (asynq
// task headers, Kafka/NATS message headers) and starts a consumer span. A
// nil map is valid — records without headers must not panic when the
// propagator injects the extracted context back into the carrier.
func SpanFromHeaders(ctx context.Context, headers map[string]string, name string) (context.Context, trace.Span) {
	carrier := propagation.MapCarrier{}
	for k, v := range headers {
		carrier[k] = v
	}
	ctx = Propagator.Extract(ctx, carrier)
	ctx, span := Tracer.Start(ctx, name)
	Propagator.Inject(ctx, carrier)
	return ctx, span
}

// InjectHeaders writes the current span context into a string map for the
// producer side of an async hop.
func InjectHeaders(ctx context.Context, headers map[string]string) {
	Propagator.Inject(ctx, propagation.MapCarrier(headers))
}

// JobMetrics adapts the Prometheus counters to service.JobObserver.
type JobMetrics struct{}

var _ service.JobObserver = JobMetrics{}

// NewJobObserver returns the Prometheus-backed job observer.
func NewJobObserver() JobMetrics { return JobMetrics{} }

// JobQueued counts an accepted job.
func (JobMetrics) JobQueued(context.Context) { JobsQueued.Inc() }

// JobFinished counts a finished job and its duration.
func (JobMetrics) JobFinished(_ context.Context, typ, status string, duration time.Duration) {
	JobsProcessed.WithLabelValues(status).Inc()
	JobDuration.WithLabelValues(typ).Observe(duration.Seconds())
}
