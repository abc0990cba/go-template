# 01 — Introduction & Principles

**In one sentence:** this repository is a *production-shaped* full-stack
application template — a Go API + worker with an embedded React SPA, built to
demonstrate, in one understandable codebase, every pattern a real service
needs: auth, RBAC, multi-tenancy, background jobs, event streaming, file
storage, audit, observability, resilience and a release pipeline.

## 1.1 What it is

A single product (a small "helpdesk + admin back-office" in demo form:
tickets with approval chains, users, organizations, analytics) that is
deliberately *small enough to read end-to-end* while containing the
machinery expected from a real service:

- two binaries — `api` (HTTP API + embedded SPA) and `worker` (queues,
  stream consumers, outbox relay);
- PostgreSQL as the system of record, Redis for queue/cache/rate limiting,
  ClickHouse as the analytics warehouse, S3/MinIO for blobs, Kafka and
  NATS JetStream as event transports, SMTP for email;
- **every external dependency is optional** — with zero configuration the
  app runs on in-memory equivalents so a newcomer can `go run ./cmd/api`
  and have a working product in seconds.

The demo domain (tickets, approvals, users, enrichment) is a *carrier* for
the patterns; it is meant to be replaced by your domain (see the
[Template Guide](15-template-guide.md)).

## 1.2 Who it is for

| Audience | What they get |
|---|---|
| A developer starting a new product | A skeleton with the boring-but-critical parts already solved (auth, RBAC, jobs, migrations, CI, Helm) |
| A team adopting a pattern | A reference implementation of each pattern: outbox, inbox/exactly-once, idempotency, breakers, ETag caching, contract tests |
| A reviewer / architect | ADRs explaining every major choice with alternatives and consequences |
| An SRE | Health probes, metrics catalog, graceful degradation rules, Helm chart, release pipeline with SBOM/signing/scanning |

## 1.3 Quality goals (and how each is met)

| Quality goal | How the architecture achieves it | Where to verify |
|---|---|---|
| **Understandability** | Strict layering, ports & adapters, ~22k lines of Go total, self-explaining comments | [04 — Components](04-components-and-code-map.md) |
| **Zero-friction onboarding** | In-memory adapters for every dependency; `docker compose up` for the full stack | [13 — Deployment](13-deployment-and-operations.md) |
| **Correctness** | Contract tests run the *same* assertions against memory and Postgres adapters; e2e suites; 33-check live walkthrough | [14 — Testing](14-testing-and-quality.md) |
| **Resilience** | Circuit breakers on every outbound call, retries with jitter, idempotency keys, transactional outbox, DLQ with replay | [12 — Observability & Reliability](12-observability-and-reliability.md) |
| **Security** | Argon2id passwords, hashed single-use tokens, RBAC on Casbin, CSRF/origin checks, security headers, `_FILE` secrets | [11 — Security](11-security.md) |
| **Operability** | `/health/*`, Prometheus metrics, OTel traces, structured logs, maintenance mode | [12 — Observability](12-observability-and-reliability.md) |
| **Shipability** | Multi-stage distroless image, Helm chart (HPA/PDB/probes), Trivy scan + SBOM + keyless signing | [13 — Deployment](13-deployment-and-operations.md) |

## 1.4 The seven ground rules

These principles are enforced by code review and CI, and every ADR traces
back to one of them.

1. **Ports & adapters, always.** Business logic in `internal/service/*`
   speaks only to Go interfaces (`internal/service/ports.go`). Postgres,
   the in-memory store, S3, SMTP, Kafka, NATS are all adapters behind those
   interfaces. ADR [0001](decisions/0001-hexagonal-architecture.md).
2. **Degrade, never die.** A missing dependency removes a capability, never
   the ability to boot: no Postgres → memory store; no Redis → in-process
   queue and in-memory rate limits; no ClickHouse → in-memory event trail;
   no S3 → in-memory blobs; no SMTP → mail logged to stdout.
   ADR [0004](decisions/0004-graceful-degradation.md).
3. **The database is the contract for data; OpenAPI is the contract for
   HTTP.** SQL lives in reviewed `.sql` files compiled to Go by sqlc; HTTP
   shapes live in `api/openapi/openapi.yaml` with generated Go types.
   ADRs [0003](decisions/0003-sqlc-over-orm.md),
   [0009](decisions/0009-openapi-contract-first.md).
4. **State machines own their transitions.** Business objects (tickets,
   jobs, invitations, tokens) expose named actions; illegal transitions are
   domain errors (`409 Conflict`), not silent updates.
5. **Every write path is idempotent or retry-safe.** Client-facing writes
   can carry `Idempotency-Key`; workers use asynq task-ID dedupe; stream
   consumption is exactly-once by inbox + sequence guards.
6. **Observability is not optional.** Every request gets a request id, a
   trace span and a RED metric; every outbound call gets a circuit breaker
   and metrics.
7. **Docs and tests are part of the definition of done.** A feature is done
   when: OpenSpec change exists, OpenAPI updated, migration + sqlc queries
   written, tests added (unit + integration + e2e as applicable),
   traceability matrices updated. ADR [0019](decisions/0019-openspec-workflow.md).

## 1.5 Constraints the design accepts

- **Single repository, two binaries** — the template targets products whose
  scale is served by a scaled monolith, not microservices
  (ADR [0002](decisions/0002-monolith-first.md)).
- **No C extensions**: pure-Go drivers only (pgx, franz-go, nats.go,
  clickhouse-go) so `CGO_ENABLED=0` produces static distroless images.
- **PostgreSQL 16 / Redis 7 / ClickHouse 24.8** are the versions the local
  stack and tests pin; anything S3-compatible works for blobs.
- **The SPA is embedded** into the `api` binary — one artifact to deploy,
  no CDN requirement (ADR [0010](decisions/0010-embed-spa.md)).

## 1.6 What is intentionally *not* here

Being explicit about scope keeps the template honest:

- No GraphQL/gRPC endpoints (OpenAPI/JSON only).
- No multi-region replication story; Postgres is the single source of truth.
- No workflow engine — approvals are a purpose-built state machine, simple
  enough to read.
- No built-in rate-limit dashboard or alerting rules — metrics are
  exported; dashboards are yours to add.
- The SPA has no attachment *upload* UI yet (the API is complete and covered
  by e2e/regression scripts); this is a known, honest gap.

Next: [02 — System Context](02-system-context.md) — the 30,000-foot view.
