## Why

The service had no metrics, no tracing, and no access log; request ids were generated but unused. Debugging latency or failures in production shape requires all three signals wired end to end.

## What Changes

- Structured request logging: one slog line per request with request id, status, and duration; slow requests escalate to warn; JSON or text via `APP_LOG_FORMAT`.
- Prometheus metrics on `/metrics`: RED middleware with bounded-cardinality labels, runtime metrics, and business job counters emitted through a new `service.JobObserver` port (Prometheus stays out of the domain).
- Distributed tracing: OpenTelemetry with OTLP/HTTP export (`APP_OTLP_ENDPOINT`); otelgin server spans, asynq task-header propagation so worker spans join the API trace, and otelpgx query spans.
- Observability compose profile: Jaeger (OTLP), Prometheus, Grafana with provisioned datasources.
- `APP_PPROF=1` mounts pprof; `/version` reports ldflags-stamped build metadata.

## Capabilities

### New Capabilities

- `observability-signals`

## Impact

- api and worker both initialize tracing and structured logging; compose services now export OTLP to Jaeger and log JSON.
