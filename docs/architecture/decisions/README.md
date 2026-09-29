# Architecture Decision Records

Decisions are numbered chronologically. Each ADR is immutable history: to
change a decision, write a new ADR that supersedes it and link both ways.

| ADR | Decision | Status |
|---|---|---|
| [0001](0001-hexagonal-architecture.md) | Hexagonal architecture (ports & adapters) with an in-memory twin adapter | Accepted |
| [0002](0002-monolith-first.md) | Monolith-first: one codebase, two binaries | Accepted |
| [0003](0003-sqlc-over-orm.md) | sqlc (typed SQL codegen) instead of an ORM | Accepted |
| [0004](0004-graceful-degradation.md) | Every dependency is optional; degrade, never die | Accepted |
| [0005](0005-sessions-vs-jwt.md) | Opaque server-side sessions for browsers; JWT grants only for API/mobile | Accepted |
| [0006](0006-argon2id-password-hashing.md) | Argon2id for password hashing | Accepted |
| [0007](0007-casbin-rbac-with-domains.md) | Casbin RBAC with domains as tenants | Accepted |
| [0008](0008-asynq-redis-jobs.md) | asynq on Redis for background jobs | Accepted |
| [0009](0009-openapi-contract-first.md) | OpenAPI contract-first with types-only codegen | Accepted |
| [0010](0010-embed-spa.md) | Embed the SPA into the api binary | Accepted |
| [0011](0011-exactly-once-stream-consumption.md) | Exactly-once consumption: inbox + per-source sequences + field ownership | Accepted |
| [0012](0012-transactional-outbox.md) | Transactional outbox for broker publishing | Accepted |
| [0013](0013-problem-json-errors.md) | RFC 9457 problem+json as the single error convention | Accepted |
| [0014](0014-idempotency-key-middleware.md) | Idempotency-Key middleware for client retries | Accepted |
| [0015](0015-rate-limiting-adapters.md) | Layered rate limiting: token bucket (memory) / sliding window (Redis) | Accepted |
| [0016](0016-circuit-breakers.md) | A circuit breaker around every outbound dependency | Accepted |
| [0017](0017-clickhouse-olap.md) | ClickHouse as a separate OLAP store; OLTP stays in Postgres | Accepted |
| [0018](0018-distroless-supply-chain.md) | Distroless images + Trivy/SBOM/cosign supply chain | Accepted |
| [0019](0019-openspec-workflow.md) | OpenSpec change-first workflow with traceability matrices | Accepted |
| [0020](0020-fulltext-search-simple.md) | Postgres tsvector with the 'simple' config for full-text search | Accepted |

## Format

Classic Nygard ADR: **Status / Context / Decision / Consequences**
(+ *Alternatives considered* where the choice was contested). Each record
is short on purpose — the code is the detailed specification; the ADR
explains why the code looks the way it does.
