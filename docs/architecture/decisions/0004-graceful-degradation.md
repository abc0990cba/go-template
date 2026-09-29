# ADR 0004 — Every Dependency Is Optional: Degrade, Never Die

**Status:** Accepted

## Context

A full-stack template with Postgres + Redis + ClickHouse + Kafka + NATS +
S3 + SMTP as *requirements* would demand a running datacenter before the
first `go run` — the fastest way to kill a template's adoption. Yet the
same template must run the real stack in production. The tension resolves
only if every dependency has a first-class in-process fallback.

## Decision

Each external dependency maps to a port with (at least) two adapters, and
bootstrap picks per environment variable presence:

| Dependency | Absent → |
|---|---|
| PostgreSQL (`APP_DATABASE_URL`) | `repository.MemoryStore` implements **all** ports |
| Redis (`APP_REDIS_ADDR`) | in-process job ticker (2 s), memory cache, memory rate limits, SSE without cross-replica bridge |
| ClickHouse (`APP_CLICKHOUSE_ADDR`) | in-memory event trail (open failure is a warning, not fatal) |
| S3 (`APP_S3_BUCKET`) | in-memory blob store (presign answers 501) |
| SMTP (`APP_SMTP_HOST`) | log mailer; dev keeps flows usable via debug-level links |
| Kafka / NATS | that side disabled with a warning; the other side unaffected |

One deliberate exception: **once `APP_DATABASE_URL` is set, Postgres is
required** — a silent memory fallback against a configured-but-broken
production database would be worse than a crash.

Equivalence is not aspirational: contract tests run the same assertions
against the memory and Postgres adapters on every CI run (ADR 0001).

## Consequences

**Positive:** onboarding is `git clone && go run ./cmd/api`; every demo,
unit test and dev loop runs without infrastructure; degradation paths are
exercised constantly instead of rotting.

**Negative:** two implementations of every repository must be kept in
lockstep (the contract tests are the price and the guarantee); memory
mode is single-process — doc'd loudly so nobody "scales" it.

## Alternatives considered

- *Docker required for dev* — common, but hostile to quick exploration
  and CI cold starts.
- *Test-only fakes, real DB for the app* — the memory store *is* that
  fake, promoted to a runnable product mode; same code, more value.
