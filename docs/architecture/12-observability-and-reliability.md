# 12 — Observability & Reliability

**In one sentence:** every request is logged, counted, timed and traced;
every outbound dependency sits behind a circuit breaker with metrics; and
the reliability toolkit (retry with jitter, bounded concurrency,
idempotency, outbox, DLQ, graceful shutdown) is small enough to read in an
afternoon.

## 12.1 The four observability signals

### Structured logs (slog)
One line per request (`method`, `path`, `status`, `duration_ms`,
`request_id`, `user_id`); `slow request` above 1 s; JSON or text via
`APP_LOG_FORMAT`. Domain events (registration, logins, job transitions,
stream applies) log with their own attributes. No third-party logger.

### Metrics (Prometheus, `app_` prefix)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `app_http_requests_total` | counter | method, route, status | RED rate |
| `app_http_request_duration_seconds` | histogram | method, route | RED duration |
| `app_http_requests_in_flight` | gauge | — | concurrency |
| `app_jobs_queued_total` / `app_jobs_processed_total{status}` | counters | status | queue throughput |
| `app_job_duration_seconds{type}` | histogram | type | per-type latency |
| `app_cb_state{name}` | gauge | name | 0 closed / 1 half-open / 2 open |
| `app_cb_transitions_total{name,from,to}` | counter | — | breaker churn |
| `app_enrichment_events_total{source,result}` | counter | — | applied/deduped/stale/parked |
| `app_outbox_pending` / `app_dead_letters_pending` | gauges | — | relay/DLQ backlog |
| `app_outbox_relay_total{result}` / `app_dead_letters_replayed_total` | counters | — | relay outcomes, recovery |
| `app_rate_limit_rejected_total{scope}` + `app_rate_limit_errors_total` | counters | — | throttling + fail-opens |
| `app_idempotency_replays_total` | counter | — | client retries served from store |

Scrape config: Prometheus pulls the api at `:8080/metrics` and the worker
at `APP_METRICS_ADDR` (compose `:9090`) every 5 s; Grafana ships with
provisioned Prometheus + Jaeger datasources (dashboards are intentionally
left to you).

### Traces (OpenTelemetry)
A server span per request (`HTTP <method>` → renamed to the route
pattern), propagated end-to-end: **HTTP → asynq task headers → Kafka/NATS
message headers → consumer spans**, with **pgx query spans** (otelpgx)
when `APP_OTLP_ENDPOINT` is set. Jaeger all-in-one in the observability
profile shows the whole chain — click a `job.updated` SSE event back to
the enqueue request.

### Health & identity endpoints

| Endpoint | Semantics |
|---|---|
| `/health/live` | process is up (liveness — never checks dependencies) |
| `/health/ready` | store ping + optional broker probes (Kafka probe pings without joining the consumer group — a non-polling member would hold partitions) |
| `/health/startup` | container startup gate (probes: startup 10 s window, then live/ready) |
| `/version` | `{version, commit, date}` — stamped at build |
| `/debug/pprof/*` | behind `APP_PPROF=1` only |
| `/docs` | Scalar API reference over the live OpenAPI spec |

## 12.2 The reliability toolkit (`internal/reliability` + middleware)

### Circuit breakers
Per-dependency named breakers (`enrichment.http`, `webhooks.http`,
`kafka.produce`, `nats.publish`): trip open after **5 consecutive
failures**, stay open **30 s**, admit exactly one probe, close on success
→ re-open on probe failure. Context cancellation never counts as failure
(shutdowns don't trip). Every transition is a metric. Calls through an
open breaker fail fast with `503 dependency temporarily unavailable`.

### Retry with full jitter
`reliability.Retry`: exponential `baseDelay * 2^attempt` capped at shift 5,
**plus full jitter**; `PermanentError{}` marks non-retryable (e.g. 4xx).
Used by enrichment fetches, stream applies (3 × 200 ms) and webhook
inline mode.

### Bounded concurrency
`reliability.RunBounded` caps worker pools (enrichment fan-out at
`APP_MAX_CONCURRENCY`, default 8) — no unbounded goroutine spawns on
user-controlled counts.

### Rate limiting (shared, two adapters)
Memory: per-key token buckets with a janitor. Redis: **sliding-window log**
in Lua (`ZREMRANGEBYSCORE` + `ZCARD` + conditional `ZADD`), returning the
true `Retry-After`; one window per second, cluster-wide. Fail-open on
limiter errors (availability over strictness) with a dedicated error
metric.

### Idempotency middleware
Covered in [09 §9.6](09-flows-business-domains.md) — reserve-then-complete
with request-hash binding, replay headers, 24 h TTL, 10-minute prune.

### Graceful shutdown
Signal context → `server.Shutdown` within `APP_SHUTDOWN_TIMEOUT` (10 s);
open SSE streams can exceed the grace, so a forced `Close` with a warning
follows; deferred closers unwind tracing, store, queue, bridges in order.
The worker shuts the asynq server and scheduler down symmetrically.

### Supervisor
Stream consumers run under `supervise()`: a fatal exit restarts the
consumer with exponential backoff 2 s → 30 s — a poisoned broker
connection degrades to log noise, not a crash loop.

## 12.3 Graceful degradation matrix (runbook cheat-sheet)

| Down dependency | Observable behavior | User impact |
|---|---|---|
| ClickHouse | warning at boot; analytics reads fall back to counters/trail | charts degrade |
| Redis | queue becomes in-process ticker (2 s); rate limits/cache/memory; SSE local-only | single-process features only; scale-out lost |
| S3 | uploads 503-safe? no — file store **falls back to memory** with warning; presign → 501 | blobs ephemeral |
| SMTP | log mailer; debug-level links | mail flows still testable |
| Kafka | that consumer side disabled with warning; NATS side unaffected | enrichment half-blind |
| NATS | symmetric to Kafka | — |
| Postgres | **fatal at boot** (by design — no silent memory fallback once you asked for real data) | — |

## 12.4 SLO starting points (k6-verified budgets)

The performance suite (`test/performance/*.js`) encodes thresholds you can
lift into SLOs:

| Scenario | Load | Threshold |
|---|---|---|
| `/health/live` smoke | 10 VU / 30 s | fail < 1%, p95 < 500 ms |
| login | 50 VU / 30 s | fail < 1%, p95 < 300 ms |
| users list | 50 VU / 30 s | p95: offset 500 / cursor 300 / **keyset 250 ms** |
| analytics summary | 20 VU / 30 s | p95 < 500 ms |
| enrichment enqueue | 5 VU / 30 s | p95 < 2 s |

The users-list budget ladder is the point: the same feature measured in
all three pagination modes shows *why* keyset pagination exists.

Next: [13 — Deployment & Operations](13-deployment-and-operations.md).
