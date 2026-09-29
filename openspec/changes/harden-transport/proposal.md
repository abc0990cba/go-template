## Why

The template claims production shape, but a review found concrete gaps: inbound HTTP requests are never traced (docs/observability.md claims otelgin, but no span middleware exists), the pgx pool runs on defaults with no knobs, the 1 MiB body cap is bypassed by chunked requests, secrets only load from plain env vars, rate limiting keys only by IP, and there is no way to shed non-admin traffic during an incident. Each gap is small; together they are the difference between "runs" and "operable under load".

## What Changes

- **Inbound tracing**: a Gin span middleware starts a server span per request (route-pattern name, W3C tracecontext extraction, status attributes, error status for 5xx). Uses the global tracer provider, so it is a no-op unless `APP_OTLP_ENDPOINT` is set — same contract as the rest of the tracing.
- **pgx pool tuning**: `APP_PG_MAX_CONNS`, `APP_PG_MIN_CONNS`, `APP_PG_CONN_MAX_LIFETIME`, `APP_PG_CONN_MAX_IDLE_TIME`, `APP_PG_HEALTH_CHECK_PERIOD`, `APP_PG_STATEMENT_TIMEOUT`, `APP_PG_IDLE_TX_TIMEOUT` wired into `postgres.Open` via a `PoolTuning` option; runtime params apply `statement_timeout` / `idle_in_transaction_session_timeout` server-side. Migrations keep their own connection (unaffected).
- **Body limit fix**: bodies are wrapped with `http.MaxBytesReader` so chunked (Content-Length-less) requests cannot bypass the cap; a shared `a.bind` helper maps the mid-read overflow to 413 problem+json instead of a generic 400.
- **HSTS**: `Strict-Transport-Security` (1 year, includeSubDomains) is emitted whenever `APP_COOKIE_SECURE=1`.
- **Secrets `_FILE`**: `APP_DATABASE_URL_FILE`, `APP_SMTP_PASSWORD_FILE`, `APP_CLICKHOUSE_PASSWORD_FILE` read the value from a file (Docker/K8s secrets pattern); setting both the direct value and the `_FILE` variant is a startup error.
- **Per-identity rate limiting**: a second limiter scope keyed by the authenticated user id (falls back to IP before auth) on the authorized API group, with its own budget (`APP_USER_RATE_RPS`/`APP_USER_RATE_BURST`) so one compromised account cannot exhaust an IP budget shared with colleagues behind the same NAT.
- **Maintenance mode**: a public `maintenance.enabled` setting flips the API into 503 problem+json with `Retry-After` for every route except login/logout and the settings API (permission-gated, so an admin can turn it off). Health, metrics, version, and the SPA stay served. The flag is memoized for a few seconds to bound store reads.
- Hygiene: the compiled `devtools` binary at the repo root is gitignored.

## Capabilities

### New Capabilities

- `transport-hardening`

## Impact

- `internal/config` grows pool + user-rate fields and a `_FILE` resolver (fail-fast validation preserved).
- `internal/platform/postgres.Open` gains an option; both `cmd/api` and `cmd/worker` pass the tuning.
- The router middleware chain grows two middlewares (tracing, maintenance); `authorized` group grows the identity limiter.
- Handlers switch `c.BindJSON` to `a.bind` for the 413 mapping — mechanical, behavior-preserving otherwise.
- Docs (`observability.md`, `reliability.md`, `.env.example`) updated to match reality; no OpenAPI contract change (503/413 are problem+json responses within existing error envelopes).
