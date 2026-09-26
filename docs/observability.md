# Observability

The service exposes three signals, all configurable from the environment.

## Logging

`slog` with text for humans (`APP_LOG_FORMAT=text`) or JSON for shippers (`json`); level via `APP_LOG_LEVEL`. Every request writes one structured line with `method`, `path`, `status`, `duration_ms`, and `request_id`; requests over one second escalate to `warn`. Worker task lifecycle is logged the same way (start/done/failed with task id).

## Metrics

`GET /metrics` serves Prometheus. Instrumentation layers:

- RED middleware: `app_http_requests_total` (method, route pattern, status), `app_http_request_duration_seconds`, `app_http_requests_in_flight` — the route label is the Gin pattern, so cardinality is bounded.
- Business counters: `app_jobs_queued_total`, `app_jobs_processed_total{status}`, `app_job_duration_seconds{type}` — emitted through the `service.JobObserver` port, so the domain stays Prometheus-free.
- Go runtime metrics via the default registry.

## Tracing

Set `APP_OTLP_ENDPOINT` (OTLP/HTTP, e.g. `localhost:4318`) to export traces. Propagation is W3C `tracecontext`:

- `otelgin` middleware creates server spans for HTTP requests.
- The asynq producer injects the current span context into task headers; the worker mux middleware extracts it, so worker spans join the API trace.
- `otelpgx` instruments every query when the store adapter runs with tracing enabled (wired in `internal/bootstrap`).

Run the local stack with `docker compose --profile observability up` — Jaeger UI at http://localhost:16686, Prometheus at http://localhost:9090, Grafana (provisioned with both datasources) at http://localhost:3000.

## Profiling And Build Info

- `APP_PPROF=1` mounts `net/http/pprof` under `/debug/pprof` — never expose it to untrusted networks.
- `GET /version` reports version/commit/date stamped at link time (`-X main.version=...`); untagged builds report `dev`.

## Rules

- New load-bearing paths get a metric or a span — preferably reusing the existing middleware.
- Labels must have bounded cardinality (patterns and statuses, never ids or emails).
- Errors are logged with context at the boundary that handles them, once.
