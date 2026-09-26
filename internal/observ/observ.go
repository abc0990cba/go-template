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

func init() {
	prometheus.MustRegister(JobsQueued, JobsProcessed, JobDuration)
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
// task headers) and starts a consumer span.
func SpanFromHeaders(ctx context.Context, headers map[string]string, name string) (context.Context, trace.Span) {
	carrier := propagation.MapCarrier(headers)
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
