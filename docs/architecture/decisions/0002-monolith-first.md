# ADR 0002 — Monolith-First: One Codebase, Two Binaries

**Status:** Accepted

## Context

The template targets products from prototype to early scale. Microservices
before a domain is understood add distributed-systems tax (network
failures, eventual consistency, deployment orchestration) with no
compensating benefit; a single giant process, however, mixes 100 ms
request handling with minutes-long batch work in a way that hurts
scaling and blast radius.

## Decision

One repository, one `service.Container`, **two binaries**:

- `cmd/api` — everything synchronous: HTTP, SPA, SSE, auth, RBAC.
- `cmd/worker` — everything asynchronous: asynq consumption, Kafka/NATS
  consumers, outbox relay, retention prunes, cron scheduling, plus its
  own metrics listener.

They are the same application assembled twice (bootstrap constructs the
container with different optional collaborators — the worker gets no
HTTP router, no mailer cache, no realtime hub). The api scales
horizontally behind the Helm HPA; the worker scales independently by
replica count (safe: asynq distributes tasks; the outbox relay uses
`SKIP LOCKED` leases; stream consumption uses group/durable semantics
plus exactly-once dedupe).

## Consequences

**Positive:** one deployable mental model, no service discovery, ACID
transactions across all domain writes, refactoring stays atomic;
independent scaling and failure isolation where it actually pays
(webhook/broker failures cannot take down login).

**Negative:** a bad deploy affects everything (mitigated by the test
pyramid and surge-1 rollouts); module boundaries inside the monolith must
be defended by review since the compiler won't stop cross-domain imports.

## Alternatives considered

- *Microservices per bounded context* — rejected for the target stage;
  the hexagonal seams make later extraction possible without rewrites.
- *Single binary doing everything* (inline workers) — exists as the
  zero-Redis dev mode (`runLocal`), but production shape keeps
  request-serving and batch work in separate processes.
