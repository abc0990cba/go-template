# Observability

The service exposes four signals — logs, metrics, traces, and profiles —
all cross-referenced and configurable from the environment.

## Logging

`slog` with text for humans (`APP_LOG_FORMAT=text`) or JSON for shippers (`json`); level via `APP_LOG_LEVEL`. Every request writes one structured line with `method`, `path`, `status`, `duration_ms`, and `request_id`; requests over one second escalate to `warn`. Worker task lifecycle is logged the same way (start/done/failed with task id).

**Log↔trace correlation:** every line emitted inside an active span also carries `trace_id` and `span_id` (the `observ.TraceHandler` wrapper both binaries install), and the access-log middleware stamps `request.id` back onto the server span. Jaeger trace → matching log lines and back is a copy-paste, not a timestamp hunt.

## Metrics

`GET /metrics` serves Prometheus. Instrumentation layers:

- RED middleware: `app_http_requests_total` (method, route pattern, status), `app_http_request_duration_seconds`, `app_http_requests_in_flight` — the route label is the Gin pattern, so cardinality is bounded.
- Business counters: `app_jobs_queued_total`, `app_jobs_processed_total{status}`, `app_job_duration_seconds{type}` — emitted through the `service.JobObserver` port, so the domain stays Prometheus-free.
- Resilience machinery: `app_cb_state`/`app_cb_transitions_total` (breakers), `app_bulkhead_in_flight`/`app_bulkhead_rejected_total` (bulkheads), `app_rate_limit_*`, `app_idempotency_replays_total`.
- Queues and streams: `app_outbox_pending`, `app_outbox_relay_total`, `app_dead_letters_pending`, `app_enrichment_*`, `app_prune_deleted_total{domain}` (retention sweeps).
- Database pool: the hand-written `PoolStatsCollector` exports `app_pgpool_*` from `pgxpool.Stat()` — acquired/idle/max connections, and `app_pgpool_waiters_total`, the saturation signal the default runtime metrics cannot show.
- Go runtime metrics via the default registry.

## Tracing

Set `APP_OTLP_ENDPOINT` (OTLP/HTTP, e.g. `localhost:4318`) to export traces. Propagation is W3C `tracecontext`:

- A built-in server-span middleware (`internal/httpapp/observability.go`, `tracing()`) starts a span per HTTP request — named after the matched route pattern, carrying method/path/status, marked error for 5xx, and continuing inbound `traceparent` headers.
- The asynq producer injects the current span context into task headers; the worker mux middleware extracts it, so worker spans join the API trace. The same injection/extract pair covers Kafka and NATS.
- `otelpgx` instruments every Postgres query; outbound HTTP (webhooks, enrichment, OAuth) runs on `otelhttp` transports; Redis commands (cache, limiter, realtime bridge) get spans via `redisotel`; ClickHouse queries propagate the span context through the driver's explicit `WithSpan` (see `tracedCtx` in the clickhouse adapter).

**Sampling:** `APP_OTLP_SAMPLER` (a ratio in (0..1], default 1) selects head sampling via `ParentBased(TraceIDRatioBased)` — children, including async consumers, keep the root's decision. Run 1 in dev; production levels are 0.05–0.25. (The next step up is tail-based sampling at an OTel Collector; this template ships the SDK-level knob.)

## Dashboards, Alerts, SLOs

The watching half is code, provisioned by the observability profile (ADR 0022):

- **Dashboards** (`deploy/grafana/dashboards/`): API overview (RED + pg pool + Go runtime), Worker & queues (jobs, outbox, dead letters, enrichment), Resilience (breakers, bulkheads, rate limits, idempotency).
- **Alert rules** (`deploy/prometheus/rules/alerts.yml`): symptom alerts plus cause alerts for every piece of machinery above — breaker open, limiter fail-open, outbox backlog, DLQ pending, worker down, blackbox probe failed. Alertmanager runs in the profile; firing alerts are always visible at localhost:9093.
- **SLOs** (`rules/slo.yml` + `docs/slo.md`): 99.9% availability and 95%-under-500ms latency with multi-window burn-rate alerts — the same numbers the k6 suite enforces.
- **Synthetic probe**: blackbox-exporter GETs `/health/live` from outside the process; `probe_success == 0` catches a broken front door that whitebox metrics cannot see.

Run the local stack with `docker compose --profile observability up` — Jaeger UI at http://localhost:16686, Prometheus at http://localhost:9090 (alerts tab), Alertmanager at http://localhost:9093, Grafana at http://localhost:3000, blackbox at http://localhost:9115.

## Profiling And Build Info

- `APP_PPROF=1` mounts `net/http/pprof` under `/debug/pprof` — never expose it to untrusted networks.
- `APP_PYROSCOPE_URL` (set `pyroscope:4040` when the observability profile is up) starts continuous profiling: CPU + heap types pushed to Pyroscope, flame graphs in Grafana. Off by default; failures degrade to a no-op.
- `GET /version` reports version/commit/date stamped at link time (`-X main.version=...`); untagged builds report `dev`.

## Rules

- New load-bearing paths get a metric or a span — preferably reusing the existing middleware.
- Labels must have bounded cardinality (patterns and statuses, never ids or emails).
- Errors are logged with context at the boundary that handles them, once.
- A dashboard change ships in the same PR as the metric it visualizes; an alert without a runbook hint in its annotations is not done.
