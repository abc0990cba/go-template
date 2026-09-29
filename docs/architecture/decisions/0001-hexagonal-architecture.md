# ADR 0001 — Hexagonal Architecture (Ports & Adapters)

**Status:** Accepted · **Date:** project inception

## Context

The template must teach and enforce a structure that survives real product
growth: business logic that can be tested without infrastructure, storage
that can start without Docker, and adapters that can be replaced without
touching domain code. Layered ("n-tier") architectures decay when the
domain layer starts importing the DB driver "just this once".

## Decision

Organize `internal/` as concentric rings with a one-way dependency rule:

- **Center:** `internal/service` — business logic + `ports.go`, the set of
  Go interfaces that define everything the domain needs (23 repository
  ports, `JobQueue`, `Cache`, `Mailer`, `EventSink`, …).
- **Adapters (outside):** `internal/platform/*` (postgres, queue, cache,
  ratelimit, filestore, mailer, kafka, nats, clickhouse, rbac, oauth) and
  `internal/repository` (the in-memory store).
- **Driving edge:** `internal/httpapp` (HTTP) and `internal/jobs` +
  `internal/platform/queue` (workers) — they translate protocols into
  service calls, never the reverse.

The rule is enforced three ways: package imports (services never import
adapters), compile-time assertions
(`var _ AuthRepo = (*repository.MemoryStore)(nil)` on both sides), and
contract tests that run identical behavioral suites against both the
memory and Postgres adapters.

## Consequences

**Positive:** business tests need zero infrastructure; the whole app runs
with `go run ./cmd/api` and no Docker; swapping a technology is one
package; the dependency direction is checkable in review.

**Negative:** a port must be touched (two adapters + contract test) when
the data model changes — a deliberate friction tax against leaky
abstractions; some passthrough code exists where an ORM would inline.

**Neutral:** `internal/model` is shared by all rings — pragmatically
accepted to avoid DTO explosion at this codebase size.

## Alternatives considered

- *Clean Architecture with use-case layer* — more ceremony than a
  two-binary template needs; ports.go already gives the isolation.
- *Standard layered (handlers → services → repositories)* — always
  decays; no forcing function for dependency direction.
- *Full DDD tactical patterns (aggregates, repositories per aggregate)* —
  the demo domain is too small to justify it; the seam is in place if a
  product grows into it.
