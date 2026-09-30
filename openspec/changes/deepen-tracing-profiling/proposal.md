## Why

Tracing existed end-to-end but the three signals did not cross-reference: logs lacked trace IDs, sampling was always-on (the costliest telemetry mistake at production volume), Redis/ClickHouse/outbound HTTP boundaries carried no spans, pgxpool saturation was invisible, and latency had no "why" (no profiles). ADR 0023 records the decision.

## What Changes

- **Log↔trace correlation**: `observ.TraceHandler` stamps `trace_id`/`span_id` on every log line inside a span; the access-log middleware stamps `request.id` back onto the server span.
- **Sampler knob**: `APP_OTLP_SAMPLER` (0..1] via `ParentBased(TraceIDRatioBased)`, validated at startup.
- **Remaining boundaries**: `otelhttp` transport on webhook/enrichment/OAuth clients; `redisotel` on cache/ratelimit/realtime clients; ClickHouse driver span propagation (explicit `WithSpan`, the driver does not read contexts implicitly).
- **pgxpool collector**: hand-written `prometheus.Collector` exporting pool stats (acquired/idle/max, waiters, acquire duration) — `app_pgpool_*`.
- **Continuous profiling**: Pyroscope behind `APP_PYROSCOPE_URL` (off by default), service + datasource in the observability profile.

## Capabilities

### Modified Capabilities

- `observability` (spans, correlation, sampling, profiling)

## Impact

- New deps: `otelhttp` (promoted to direct), `redisotel`, `pyroscope-go`; go-redis bumped to v9.22. Tests for the handler, sampler mapping, and bulkhead-adjacent metrics; env docs in `.env.example`.
