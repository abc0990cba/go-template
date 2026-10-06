# Project Structure

A map of the repository: what each top-level directory is for and where to
find things. For the *reasoning* behind this layout (package-by-feature
vertical slices, why there is no `pkg/`), read
[architecture/04-components-and-code-map.md](../architecture/04-components-and-code-map.md)
and [ADR 0021](../architecture/decisions/0021-package-by-feature.md); the
frontend layering contract is [`web/AGENTS.md`](../../web/AGENTS.md).

## Top-level layout

| Path | What it is | Key entry points |
|---|---|---|
| `cmd/` | Binaries — one subdirectory each, thin `main.go` only | `api`, `worker`, `migrate`, `seed`, `seed-demo`, `devtools` ([reference](cli-tools.md)) |
| `internal/` | **All** application code; nothing here is importable by external projects | see below |
| `api/openapi/` | The OpenAPI contract — the single source of truth for the HTTP API; embedded into the binary and served at `/openapi.yaml` | `openapi.yaml`, `embed.go` |
| `migrations/postgres/` | golang-migrate pairs (`NNNNNN_name.up.sql` / `.down.sql`), auto-applied at boot | [database guide](../guides/database.md) |
| `migrations/clickhouse/` | Plain SQL migrations for the analytics warehouse, applied at startup | [analytics guide](../guides/analytics.md) |
| `queries/` | sqlc query files (`*.sql`) — compiled into typed Go by `make generate` | `sqlc.yaml` at repo root |
| `web/` | The React SPA (Vite); built output `web/dist/` is embedded into the Go binary | [`web/README.md`](../../web/README.md), [`web/AGENTS.md`](../../web/AGENTS.md) |
| `deploy/` | Deployment assets: `ansible/` (VPS), `helm/` (K8s), `nomad/`, `gitops/`, `prometheus/`, `grafana/`, `alertmanager/` | [`deploy/README.md`](../../deploy/README.md), [deployment hub](../deployment/README.md) |
| `scripts/` | Developer shell scripts (dev loop, chaos, kind demo, regression walkthrough) | [`scripts/README.md`](../../scripts/README.md) |
| `test/` | Cross-cutting tests: `integration/` (testcontainers), `e2e/` (build tag), `performance/` (k6) | [`test/README.md`](../../test/README.md) |
| `docs/` | This documentation system | [docs map](../README.md) |
| `openspec/` | Change-management archive: one folder per landed feature (proposal / specs / tasks) | [openspec process](openspec.md) |
| `.github/workflows/` | CI (PR gates), release (multi-arch images, signing, SBOM), DAST, release drafter | [CI/CD reference](../deployment/ci-cd.md) |

Root files worth knowing: `Makefile` (every workflow — [reference](make-targets.md)),
`docker-compose.yml` (+ `brokers`/`chaos`/`smoke` overlays — [docker guide](../deployment/docker.md)),
`Dockerfile` (3-stage build, embeds `web/dist`), `sqlc.yaml`, `.air.toml`
(hot reload), `.golangci.yml`, `lefthook.yml` (git hooks), `renovate.json`
(pins codegen tool versions), `.env.example` ([configuration reference](configuration.md)).

## Inside `internal/` — package by feature

Business features are **vertical slices**: each owns its service logic, HTTP
handlers, and tests, and exposes itself through a `RegisterHTTP(...)` function
that the router mounts. Cross-slice calls go through service ports, not
imports of another slice's internals.

| Area | Packages | Purpose |
|---|---|---|
| Foundation | `kernel/` | Shared domain vocabulary: sentinel errors, IDs, pagination, idempotency contract |
| Composition | `bootstrap/` | DI container (`Container`), store selection, demo seeders |
| Configuration | `config/` | Env parsing + strict validation ([reference](configuration.md)) |
| HTTP | `httpapp/`, `httpx/`, `api/` | Gin router + middleware, problem+json/ETag helpers, generated API types |
| Identity | `auth/`, `users/`, `sessions`* | Password auth, lockout, OAuth identities, profile |
| Access | `rbac/`, `mfa/`, `tokens/`, `apikeys/`, `invitations/` | Casbin RBAC, TOTP, JWT grants, API keys, org invitations |
| Business | `tickets/`, `jobs/`, `export/`, `files/`, `analytics/`, `search/` | Domain features |
| Platform services | `audit/`, `settings/`, `notifications/`, `email/`, `webhooks/` | Cross-cutting product services |
| Integration | `streams/`, `realtime/`, `webhooks/` | Kafka/NATS ingestion, SSE/WS hub, webhook fan-out |
| Reliability | `reliability/`, `cache/` | Circuit breakers, bulkheads, retry; read-through cache port |
| Observability | `observ/` | Metrics, tracing, profiling, log redaction |
| Adapters | `platform/` | Concrete implementations: `postgres/` (pgx + sqlc), `cache/` (Redis), `clickhouse/`, `filestore/` (S3), `queue/` (asynq), `mailer/` (SMTP), `oauth/`, `kafka/`, `nats/`, `ratelimit/`, `rbac/` (casbin), `hibp/`, `revocations/` |
| Dev fallback | `repository/` | In-memory store adapter — same contract as Postgres, asserted by tests |

\* sessions live under `auth/`; see [guides/auth.md](../guides/auth.md).

The pattern to internalize: **ports and adapters**. A feature defines an
interface (port); `platform/` and `repository/` provide interchangeable
adapters; the store-contract test suite pins both to one behavior
([testing overview](../testing/README.md)). When you add a feature, you add
a slice under `internal/`, queries under `queries/`, a migration pair under
`migrations/postgres/`, and regenerate — the full recipe is in
[architecture/15-template-guide.md](../architecture/15-template-guide.md).

## Files that require regeneration

Never hand-edit: `internal/api/api.gen.go` (oapi-codegen),
`internal/platform/postgres/db/` (sqlc), `web/src/shared/api/schema.gen.ts`
(openapi-typescript), `web/src/routeTree.gen.ts` (TanStack Router),
`web/dist/**` (Vite build). All are produced by `make generate` /
`make web-build`; CI fails if any drift from their sources.
