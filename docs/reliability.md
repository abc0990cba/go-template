# Reliability

Required production patterns:

- graceful shutdown
- context propagation
- explicit timeouts
- bounded concurrency
- retry limits
- exponential backoff with jitter
- no infinite retries
- circuit breaker for unstable external dependencies
- dead-letter handling for jobs
- idempotency keys for sensitive commands
- health endpoints

Health endpoints:

- `/health/live`
- `/health/ready` — pings the store, plus Kafka/NATS when the streams are configured
- `/health/startup`

Implemented beyond the checklist:

- **Circuit breakers** — `internal/reliability/circuitbreaker.go` wraps every outbound call (external enrichment HTTP, webhook delivery, broker publishes). Consecutive failures trip it open; one half-open probe after `APP_CB_OPEN_TIMEOUT` closes or re-opens it. `app_cb_state{name}` and `app_cb_transitions_total` expose the state machine on `/metrics`.
- **Dead letters with replay** — poison and unprocessable messages park in `dead_letters` (admin API + Streams page, audit-logged replay) instead of blocking their stream; see [docs/enrichment-streams.md](enrichment-streams.md).
- **Idempotency keys** — `Idempotency-Key` on POST routes replays stored responses for client retries, settled by a database-level reservation so concurrent double-submits cannot both proceed.
- **Distributed rate limiting** — the `/api` surface shares one Redis sliding-window budget across api processes, with per-process token buckets as the fallback and fail-open on limiter outages.

The app should fail predictably and recover cleanly.
