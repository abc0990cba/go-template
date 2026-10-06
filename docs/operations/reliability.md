# Reliability

Required production patterns:

- graceful shutdown
- context propagation
- explicit timeouts
- bounded concurrency
- bulkheads per outbound dependency
- retry limits
- exponential backoff with jitter
- no infinite retries
- circuit breaker for unstable external dependencies
- dead-letter handling for jobs
- idempotency keys for sensitive commands
- health endpoints
- scheduled retention sweeps

Health endpoints:

- `/health/live`
- `/health/ready` — pings the store, plus Kafka/NATS when the streams are configured
- `/health/startup`

Implemented beyond the checklist:

- **Circuit breakers** — `internal/reliability/circuitbreaker.go` wraps every outbound call (external enrichment HTTP, webhook delivery, broker publishes). Consecutive failures trip it open; one half-open probe after `APP_CB_OPEN_TIMEOUT` closes or re-opens it. `app_cb_state{name}` and `app_cb_transitions_total` expose the state machine on `/metrics`.
- **Bulkheads** (ADR 0025) — `internal/reliability/bulkhead.go` bounds concurrent calls per outbound dependency (`APP_BULKHEAD_LIMIT`, `APP_BULKHEAD_ACQUIRE_TIMEOUT`). A slow-but-alive dependency sheds excess callers in milliseconds instead of stacking goroutines for the full client timeout; rejections are backpressure (`ErrBulkheadFull`, counted in `app_bulkhead_rejected_total`) and never trip the breaker — bulkhead outermost, breaker innermost. Enrichment HTTP and webhook delivery are guarded; queue execution already bounds jobs at worker level.
- **Retention sweeps** — the worker prunes expired idempotency keys (10 min), GDPR export ZIPs (hourly; 7-day window), and exactly-once inbox rows (`APP_RETENTION_PROCESSED_EVENTS`, default 30d — kept longer than any broker redelivery delay so dedupe never outlives its table); ClickHouse events carry a 90-day TTL (`migrations/clickhouse/002_events_ttl.sql`). Swept rows count toward `app_prune_deleted_total{domain}`.
- **Chaos lab** — fault injection through Toxiproxy (`docker-compose.chaos.yml`, `make chaos-*`) rehearses degradation on demand: pool saturation under Postgres latency, the Redis graceful-degradation ladder, breaker-vs-bulkhead choreography on slow endpoints. Scenarios with hypotheses and expected observations live in [docs/operations/chaos.md](chaos.md).
- **Dead letters with replay** — poison and unprocessable messages park in `dead_letters` (admin API + Streams page, audit-logged replay) instead of blocking their stream; see [docs/guides/enrichment-streams.md](../guides/enrichment-streams.md).
- **Idempotency keys** — `Idempotency-Key` on POST routes replays stored responses for client retries, settled by a database-level reservation so concurrent double-submits cannot both proceed.
- **Distributed rate limiting** — the `/api` surface shares one Redis sliding-window budget across api processes, with per-process token buckets as the fallback and fail-open on limiter outages. Authenticated calls additionally pay from a **per-principal budget** (`user` scope, keyed by user id or API-key subject), so one hot account cannot exhaust the budget of everyone behind a shared NAT IP.
- **Bounded request bodies** — `Content-Length`-declared oversize bodies are rejected before handlers run, and chunked requests are capped by `http.MaxBytesReader` with the overflow surfaced as 413 problem+json (`a.bind`).
- **Connection-pool tuning** — `APP_PG_*` knobs size the pgx pool (max/min conns, conn lifetime/idle, health check period) and enforce `statement_timeout` / `idle_in_transaction_session_timeout` server-side; see `.env.example`.
- **Maintenance mode** — the public `maintenance.enabled` setting flips every API route to 503 problem+json with `Retry-After` (memoized for a few seconds per replica). Login, logout, and the permission-gated settings API stay reachable so an admin can sign in and flip the switch back; health, metrics, version, and the SPA keep serving.
- **HSTS + file secrets** — `Strict-Transport-Security` rides on every response when `APP_COOKIE_SECURE=1`, and sensitive env vars (`APP_DATABASE_URL`, `APP_SMTP_PASSWORD`, `APP_CLICKHOUSE_PASSWORD`) accept a `_FILE` variant pointing at a mounted secret file.

The app should fail predictably and recover cleanly.
