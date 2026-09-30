# Go Template Playground

Production-shaped Go application template for learning and reuse.

The project is intentionally small enough to understand, but it includes the patterns expected from a real service:

- **RBAC on Casbin** — roles with inheritance, resource:action permission matrix, raw policy editor, and multi-tenancy via Casbin domains (organizations) with per-tenant grants and a scope switcher
- **Organizations** — first-class tenants with members, tenant-scoped roles, and org-scoped API access (`X-Org-Id`)
- **Audit log** — immutable who/what/when records with before/after diffs, IP, and request id, plus an admin page with filters
- **Settings & feature flags** — typed key-value store in the database with a settings UI, public flags for pre-login pages, and read-through caching
- **Notifications** — in-app notification center with an unread badge, live SSE toasts, and mark-read flows
- **Email flows** — password reset and email verification with single-use hashed tokens; SMTP or Mailpit in dev, log fallback without config
- **API keys** — personal `gtpl_…` Bearer tokens with scope narrowing (`obj:act`), shown once, revocable
- **Impersonation** — admin "sign in as" with a visible banner, audited start/stop, and user notification
- **Webhooks** — event subscriptions with HMAC-SHA256 signatures, queue-backed delivery and retries, delivery log with a test-send button
- **Event streams** — user enrichment merged from two external systems, Kafka (`user-profile.v1`) and NATS JetStream (`users.contact.v1`), with exactly-once application (inbox dedupe + per-source sequence guards + row-locked field ownership), a transactional outbox relay, and a dead-letter queue with an admin replay console — see [docs/enrichment-streams.md](docs/enrichment-streams.md)
- **Resilience toolkit** — circuit breakers around every outbound call (`app_cb_state` metrics), Redis-backed distributed rate limiting with in-memory fallback, and `Idempotency-Key` middleware that turns client retries into replays
- **Local stream tooling** — `make demo-up` brings up Redpanda + NATS; `cmd/devtools` publishes events into both brokers, reads relayed output, and drives the breaker / rate-limit / idempotency demos (`make demo-enrichment`, `demo-dlq`, `demo-breaker`, …)
- **Tickets & approvals** — a request workflow in the spirit of Jira: a state machine with distinct "withdrawn by requester" vs "rejected by approver" outcomes, configurable multi-step approval chains per ticket type, role-guarded decisions, and separate requester ("My Tickets") and approver ("Approvals") interfaces
- **Client app vs admin console** — the SPA is split into a user-facing workspace at `/` (top navbar, personal dashboard, tickets) and a back-office console under `/admin/*` (sidebar shell, system pages), gated by the `admin_console:access` permission on the route and re-checked by every admin API
- **i18n** — English/Russian UI toggle (EN shell + RU translations)
- **Two-factor authentication (TOTP)** — QR enrollment, a challenge step in login, single-use hashed recovery codes, an `mfa.required` policy flag, and admin reset — see [docs/mfa.md](docs/mfa.md)
- **JWT token grants** — OAuth-style password/mfa/refresh grants with rotation and reuse-detection families for mobile & API clients — see [docs/token-auth.md](docs/token-auth.md)
- **Org invitations** — email offers with single-use tokens; acceptance lands membership plus the domain role in one step — see [docs/invitations.md](docs/invitations.md)
- **File storage & ticket attachments** — S3/MinIO (or in-memory) blobs, checksummed uploads, permission-scoped downloads, presigned URLs — see [docs/files.md](docs/files.md)
- **GDPR data export** — a queued job builds a ZIP of per-entity JSON, owner-only downloads, 7-day retention — see [docs/data-export.md](docs/data-export.md)
- **Caching & search** — read-through cache (memory/Redis, singleflight) for hot reads, ETag/304 on heavy lists, permission-scoped full-text search — see [docs/caching-search.md](docs/caching-search.md)
- **Transport hardening** — inbound server spans, pgx pool tuning, chunked-safe body caps, HSTS, `_FILE` secrets, per-identity rate limits, maintenance mode — see [docs/reliability.md](docs/reliability.md)
- **Deployment** — Helm chart (HPA/PDB/probes/secrets mounts) with an SLO-gated canary option (Argo Rollouts, auto-rollback), GitOps manifests, a Nomad alternative, and a signed, scanned, SBOM'd, SLSA-attested release pipeline — see [docs/deployment.md](docs/deployment.md), [docs/orchestrators.md](docs/orchestrators.md), [docs/gitops.md](docs/gitops.md)
- **Application map** — every component with file paths and a hand-check per piece: [docs/application-overview.md](docs/application-overview.md)
- **Regression** — `scripts/regression-walkthrough.sh` drives every feature against a live stack; the full gauntlet is documented in [docs/regression-checklist.md](docs/regression-checklist.md)
- **Rich demo data** — 500 users, 3 organizations, 120 tickets across the whole approval lifecycle, 90 days of events (12 types with property dimensions), audit entries, notifications, keys, and webhook history on first launch
- Gin HTTP API
- React admin SPA (Vite + TanStack Router/Query/Table, shadcn/ui, Tailwind v4) embedded in the binary, with Server-Sent Events for live updates
- OpenAPI contract
- OpenSpec workflow
- CI (GitHub Actions) with vet, golangci-lint, and race-enabled tests
- PostgreSQL persistence (pgx + sqlc + migrations) behind service ports
- Redis/asynq job queue with a real worker process and idempotent enqueueing
- Observability: structured logs with trace↔log correlation, Prometheus metrics (RED, pool stats, resilience machinery), OpenTelemetry traces across every boundary (HTTP, asynq, Kafka/NATS, pgx, Redis, ClickHouse, outbound HTTP), configurable sampling, continuous profiling (Pyroscope), dashboards + alert rules + SLO burn-rate alerts + Alertmanager shipped as code, blackbox synthetic checks — see [docs/observability.md](docs/observability.md) and [docs/slo.md](docs/slo.md)
- PostgreSQL migrations and sqlc query layout
- Redis/asynq-ready background job design
- ClickHouse analytics schema
- Unit, integration, e2e, and performance test structure
- Reliability rules: graceful shutdown, context propagation, bounded concurrency, retry limits, and health checks

## Documentation

- **[docs/architecture/](docs/architecture/)** — the master architecture set: C4 diagrams (context → containers → components), every flow as a sequence/state diagram (auth, MFA, JWT rotation, approvals, streams/outbox/DLQ, files, export, webhooks, idempotency), the data model ER, the full technology stack with rationale, security model, operations, testing strategy, and a template adoption guide.
- **[docs/architecture/decisions/](docs/architecture/decisions/)** — 20 ADRs recording every significant architectural choice (hexagonal architecture, monolith-first, sqlc over ORM, sessions vs JWT, exactly-once consumption, transactional outbox, distroless supply chain, …).
- Feature guides: [application overview](docs/application-overview.md), [quickstart](docs/quickstart.md), [tickets](docs/tickets.md), [MFA](docs/mfa.md), [token auth](docs/token-auth.md), [RBAC](docs/rbac.md), [invitations](docs/invitations.md), [files](docs/files.md), [data export](docs/data-export.md), [enrichment streams](docs/enrichment-streams.md), [caching & search](docs/caching-search.md), [jobs](docs/jobs.md), [database](docs/database.md), [observability](docs/observability.md), [SLOs](docs/slo.md), [reliability](docs/reliability.md), [chaos lab](docs/chaos.md), [deployment](docs/deployment.md), [orchestrators](docs/orchestrators.md), [GitOps](docs/gitops.md), [frontend](docs/frontend.md), [testing](docs/testing.md), [regression checklist](docs/regression-checklist.md).

## Demo accounts

Seeded by the demo dataset (also shown as a one-click table on the login page when the
`demo_mode` flag is on). Passwords are for local verification only.

| Account | Password | Roles | Signs into | What to check |
|---|---|---|---|---|
| `demo@go-template.local` | `demo-password-123` | superadmin | both | Everything; the compose/quickstart login |
| `demo-u-0004@demo.example` | `password123` | requester | client app only | Files and tracks tickets; `/admin` redirects away; can withdraw a ticket in any active state |
| `demo-u-0005@demo.example` | `password123` | approver | client app only | The Approvals inbox: decides the final step of a chain |
| `demo-u-0006@demo.example` | `password123` | requester + approver | client app only | Both client sides of the flow |
| `demo-u-0002@demo.example` | `password123` | manager | admin + approvals | Admin console and a filled approvals inbox (first chain step) |
| `demo-u-0001@demo.example` | `password123` | admin | admin + approvals | Full console, org-wide ticket tabs, decides any step |
| `demo-u-0003@demo.example` | `password123` | viewer | admin (read-only) | Read-only back-office access |
| `demo-u-0010@demo.example` | `password123` | manager (Acme org) | admin, org scope | Sees only Acme's tickets — switch the org scope |

Full flow to try: sign in as **u-0004**, file and submit an Expense ticket → **u-0002** approves
the manager step → **u-0005** approves the final step → back as **u-0004**, deliver the result.

## Run Locally

```bash
make docker-up          # full stack
# or the hybrid dev mode - see docs/quickstart.md
docker compose up -d postgres redis
APP_DATABASE_URL="postgres://app:app@127.0.0.1:55432/app?sslmode=disable" \
APP_REDIS_ADDR="127.0.0.1:6379" \
APP_DEMO_EMAIL="demo@go-template.local" \
APP_DEMO_PASSWORD="demo-password-123" \
APP_COOKIE_SECURE=0 \
go run ./cmd/api
```

Demo account (seeded at startup when `APP_DEMO_EMAIL`/`APP_DEMO_PASSWORD` are set): `demo@go-template.local` / `demo-password-123`. With `APP_DEMO_SEED=1` the api also loads a demo dataset — 120 users (ten of them login-able with `password123`), OAuth identities, jobs in every status, reports, enrichment profiles, and 30 days of event history for the charts. Re-seed anytime with `make seed-demo` / `make seed-demo-reset`. Full runbook: [docs/quickstart.md](docs/quickstart.md).

Open:

- App UI: http://localhost:8080
- API docs: http://localhost:8080/docs
- Metrics: http://localhost:8080/metrics
- Version: http://localhost:8080/version
- Health: http://localhost:8080/health/live

Sign in with the demo credentials, then explore: the dashboard (KPIs, 30-day charts, live event feed), users with switchable offset/cursor/keyset pagination, the jobs queue with live status updates over SSE, enrichment, reports, analytics, and the in-app Getting Started tour. Each page explains itself via the banner on top. Frontend details: [docs/frontend.md](docs/frontend.md).

## Development Commands

```bash
make vet
make lint
make test
make coverage
make test-performance
make web-lint      # oxlint + oxfmt check for the SPA
make web-test      # vitest
make web-dev       # Vite dev server with API proxy
make web-build     # build web/dist for embedding
make seed-demo     # load the demo dataset
make docker-up
```

The default run uses an in-memory repository so the app can be explored immediately. Set `APP_DATABASE_URL` and `APP_REDIS_ADDR` (see `.env.example`) — or run `make docker-up` — for PostgreSQL (pgx + sqlc + startup migrations) and the Redis/asynq queue with a real worker process. Add `--profile observability` for Jaeger, Prometheus, and Grafana; see `docs/observability.md`. ClickHouse analytics power the event charts when `APP_CLICKHOUSE_ADDR` is set; without it the app degrades to the in-memory event trail. For the enrichment streams, `make demo-up` adds Redpanda (Kafka API) and NATS with JetStream; the workers start consuming and the outbox relay starts publishing automatically.

## Project Rules

- Start each feature with an OpenSpec change.
- Update OpenAPI for HTTP behavior.
- Update SQL migrations and sqlc queries for database behavior.
- Add tests and performance coverage for each user-facing or load-bearing feature.
- Update traceability matrices before a feature is considered complete.
