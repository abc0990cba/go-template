## Why

Performance targets were documented but nothing could execute them: no seeded data, no scenario scripts beyond the health smoke, no hot-path benchmarks.

## What Changes

- `cmd/seed`: bulk-insert synthetic users via pgx COPY with spread timestamps (requires APP_DATABASE_URL); `make seed-users`.
- k6 scenarios with the documented thresholds: login burst, users list in all three pagination modes (MODE env), analytics read, enrichment enqueue; shared helper for base URL and login.
- Go benchmarks for Argon2id hash/verify (measured ~20ms and ~67MB per hash on the reference laptop — login is CPU-bound).
- Makefile targets: perf-login, perf-users (MODE=...), perf-analytics, perf-enrichment; docs/performance.md rewritten with the runbook; matrix marks scripts Implemented with a runs table to fill per environment.

## Capabilities

### New Capabilities

- none (strengthens the existing performance documentation into an executable harness)
