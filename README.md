<div align="center">

# Go Template Playground

**A production-shaped Go + React application template — for learning, reuse, and as a starter for real products.**

Everything a real internal product needs on day one: authentication with MFA, RBAC with multi-tenancy,
a ticket approval workflow, background jobs, Kafka/NATS event streams, webhooks, file storage,
analytics, observability, and Helm charts — in **one binary** you can read end to end.

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-pgx%20%2B%20sqlc-4169E1?logo=postgresql&logoColor=white)
![Redis](https://img.shields.io/badge/Redis-asynq%20queue-DC382D?logo=redis&logoColor=white)
![ClickHouse](https://img.shields.io/badge/ClickHouse-analytics-FFCC01?logo=clickhouse&logoColor=black)
![Docker](https://img.shields.io/badge/Docker-compose%20%7C%20Helm%20%7C%20Nomad-2496ED?logo=docker&logoColor=white)

![The landing page](docs/screenshots/landing-hero.png)

*The prerendered SEO landing at `/` — one of two UIs the binary serves (available in English, Russian, French, and German).*

**New here?** Start with the **[documentation map](docs/README.md)** — reading order, tracks by role, and a find-by-task table. For the one-command VPS deploy, see [Deploy](#-deploy-to-a-vps-one-command) below.

</div>

---

## ✨ What's inside

The project is intentionally small enough to understand, but it includes the patterns expected from a real service — the plumbing is done, so your first commit is a feature:

- **One binary** — the React app is embedded into the Go binary: one container, one port, no CORS, no separate hosting
- **Client app vs admin console** — a user-facing workspace at `/dashboard` (top navbar, personal dashboard, tickets) and a back-office console under `/admin/*` (sidebar shell, system pages), gated by the `admin_console:access` permission and re-checked by every admin API
- **Tickets & approvals** — a Jira-shaped request workflow with configurable multi-step approval chains
- **Event streams** — Kafka + NATS JetStream consumers with exactly-once application and a dead-letter console
- **Observability** — traces, metrics, SLO burn-rate alerts, and dashboards shipped as code
- **Rich demo data** — 500 users, 3 organizations, 120 tickets across the whole approval lifecycle, 90 days of events, audit entries, notifications, keys, and webhook history on first launch

## 📸 Screenshots

Captured from the seeded demo dataset with the English UI.

| Client workspace — dashboard | Client workspace — tickets |
|---|---|
| ![Dashboard](docs/screenshots/dashboard.png) | ![Tickets](docs/screenshots/tickets.png) |

| Ticket with approval chain | Approver inbox |
|---|---|
| ![Ticket detail](docs/screenshots/ticket-detail.png) | ![Approvals](docs/screenshots/approvals.png) |

| Sign in with one-click demo accounts | Admin console — dashboard |
|---|---|
| ![Login](docs/screenshots/login.png) | ![Admin dashboard](docs/screenshots/admin-dashboard.png) |

| Admin console — users | Admin console — jobs |
|---|---|
| ![Users](docs/screenshots/admin-users.png) | ![Jobs](docs/screenshots/admin-jobs.png) |

| Admin console — analytics | Admin console — raw Casbin policies |
|---|---|
| ![Analytics](docs/screenshots/admin-analytics.png) | ![Policies](docs/screenshots/admin-policies.png) |

**Admin console — audit log** (immutable who/what/when records with before/after diffs)

![Audit log](docs/screenshots/admin-audit.png)

<details>
<summary><strong>The full landing page</strong> (all 18 feature cards, click to expand)</summary>

![The full landing page](docs/screenshots/landing-full.png)

</details>

## 🧩 Features

### Authentication & identity

- **Sessions, OAuth, MFA** — cookie sessions, OAuth identity links, TOTP two-factor (QR enrollment, login challenge step, single-use hashed recovery codes, an `mfa.required` policy flag, admin reset) — [docs/guides/mfa.md](docs/guides/mfa.md)
- **Email flows** — password reset and email verification with single-use hashed tokens; SMTP or Mailpit in dev, log fallback without config
- **API keys** — personal `gtpl_…` Bearer tokens with scope narrowing (`obj:act`), shown once, revocable
- **JWT token grants** — OAuth-style password/mfa/refresh grants with rotation and reuse-detection families for mobile & API clients — [docs/guides/token-auth.md](docs/guides/token-auth.md)
- **Impersonation** — admin "sign in as" with a visible banner, audited start/stop, and user notification
- **Org invitations** — email offers with single-use tokens; acceptance lands membership plus the domain role in one step — [docs/guides/invitations.md](docs/guides/invitations.md)

### Authorization & multi-tenancy

- **RBAC on Casbin** — roles with inheritance, a resource:action permission matrix, a raw policy editor, and an audit trail behind every state change
- **Organizations** — first-class tenants via Casbin domains: members, tenant-scoped roles, org-scoped API access (`X-Org-Id`), and a scope switcher
- **Settings & feature flags** — typed key-value store in the database with a settings UI, public flags for pre-login pages, and read-through caching

### Tickets & approvals

- **Request workflow in the spirit of Jira** — a state machine with distinct "withdrawn by requester" vs "rejected by approver" outcomes, configurable multi-step approval chains per ticket type, role-guarded decisions, and separate requester ("My Tickets") and approver ("Approvals") interfaces — [docs/guides/tickets.md](docs/guides/tickets.md)

### Platform services

- **Background jobs** — a real worker process on Redis/asynq with idempotent enqueueing, retries, and a jobs queue with live status updates over Server-Sent Events
- **Event streams** — user enrichment merged from two external systems, Kafka (`user-profile.v1`) and NATS JetStream (`users.contact.v1`), with exactly-once application (inbox dedupe + per-source sequence guards + row-locked field ownership), a transactional outbox relay, and a dead-letter queue with an admin replay console — [docs/guides/enrichment-streams.md](docs/guides/enrichment-streams.md)
- **Webhooks** — event subscriptions with HMAC-SHA256 signatures, queue-backed delivery and retries, a delivery log with a test-send button
- **Notifications** — in-app notification center with an unread badge, live SSE toasts, and mark-read flows
- **File storage & ticket attachments** — S3/MinIO (or in-memory) blobs, checksummed uploads, permission-scoped downloads, presigned URLs — [docs/guides/files.md](docs/guides/files.md)
- **GDPR data export** — a queued job builds a ZIP of per-entity JSON, owner-only downloads, 7-day retention — [docs/guides/data-export.md](docs/guides/data-export.md)
- **Caching & search** — read-through cache (memory/Redis, singleflight) for hot reads, ETag/304 on heavy lists, permission-scoped full-text search — [docs/guides/caching-search.md](docs/guides/caching-search.md)

### Operations & reliability

- **Resilience toolkit** — circuit breakers around every outbound call (`app_cb_state` metrics), Redis-backed distributed rate limiting with in-memory fallback, and `Idempotency-Key` middleware that turns client retries into replays
- **Transport hardening** — inbound server spans, pgx pool tuning, chunked-safe body caps, HSTS, `_FILE` secrets, per-identity rate limits, maintenance mode — [docs/operations/reliability.md](docs/operations/reliability.md)
- **Observability** — structured logs with trace↔log correlation, Prometheus metrics (RED, pool stats, resilience machinery), OpenTelemetry traces across every boundary (HTTP, asynq, Kafka/NATS, pgx, Redis, ClickHouse, outbound HTTP), configurable sampling, continuous profiling (Pyroscope), dashboards + alert rules + SLO burn-rate alerts + Alertmanager shipped as code, blackbox synthetic checks — [docs/operations/observability.md](docs/operations/observability.md), [docs/operations/slo.md](docs/operations/slo.md)
- **Deployment** — Helm chart (HPA/PDB/probes/secrets mounts) with an SLO-gated canary option (Argo Rollouts, auto-rollback), GitOps manifests, a Nomad alternative, a one-command self-hosted **VPS stack** (Ansible + Nomad + Caddy + own CI/registry + monitoring + backups), and a signed, scanned, SBOM'd, SLSA-attested release pipeline — [docs/deployment/vps/QUICKSTART.md](docs/deployment/vps/QUICKSTART.md), [docs/deployment/README.md](docs/deployment/README.md), [docs/deployment/orchestrators.md](docs/deployment/orchestrators.md), [docs/deployment/gitops.md](docs/deployment/gitops.md)
- **Reliability rules throughout** — graceful shutdown, context propagation, bounded concurrency, retry limits, and health checks

### Frontend & developer experience

- **SEO landing** — `/` serves a build-time-prerendered marketing page (complete HTML + canonical/OG/JSON-LD, sitemap/robots, brotli, immutable asset caching — ADR 0026), in four languages: English, Russian, French, German
- **i18n** — English/Russian in-app toggle (EN shell + RU translations)
- **Modern SPA stack** — React 19, Vite, TanStack Router/Query/Table, shadcn/ui, Tailwind v4, Gin HTTP API behind the same origin, OpenAPI contract with generated TypeScript clients
- **Local stream tooling** — `make demo-up` brings up Redpanda + NATS; `cmd/devtools` publishes events into both brokers, reads relayed output, and drives the breaker / rate-limit / idempotency demos (`make demo-enrichment`, `demo-dlq`, `demo-breaker`, …)
- **Regression** — `scripts/regression-walkthrough.sh` drives every feature against a live stack; the full gauntlet is documented in [docs/testing/regression-checklist.md](docs/testing/regression-checklist.md)
- **Application map** — every component with file paths and a hand-check per piece: [docs/guides/application-overview.md](docs/guides/application-overview.md)
- **Engineering workflow** — OpenSpec change proposals, sqlc migrations, CI (GitHub Actions) with vet, golangci-lint, and race-enabled tests, unit/integration/e2e/performance test structure, PostgreSQL migrations and sqlc query layout, ClickHouse analytics schema

## 🚀 Run locally

```bash
make docker-up          # full stack
# or the hybrid dev mode - see docs/getting-started/quickstart.md
docker compose up -d postgres redis
APP_DATABASE_URL="postgres://app:app@127.0.0.1:55432/app?sslmode=disable" \
APP_REDIS_ADDR="127.0.0.1:6379" \
APP_DEMO_EMAIL="demo@go-template.local" \
APP_DEMO_PASSWORD="demo-password-123" \
APP_COOKIE_SECURE=0 \
go run ./cmd/api
```

Demo account (seeded at startup when `APP_DEMO_EMAIL`/`APP_DEMO_PASSWORD` are set): `demo@go-template.local` / `demo-password-123`. With `APP_DEMO_SEED=1` the api also loads a demo dataset — 500 users (ten of them login-able with `password123`), 3 organizations, 120 tickets, jobs in every status, reports, enrichment profiles, and 90 days of event history for the charts. Re-seed anytime with `make seed-demo` / `make seed-demo-reset`. Full runbook: [docs/getting-started/quickstart.md](docs/getting-started/quickstart.md).

Then open:

| URL | What it is |
|---|---|
| http://localhost:8080 | App UI (landing → workspace) |
| http://localhost:8080/docs | API docs |
| http://localhost:8080/metrics | Prometheus metrics |
| http://localhost:8080/version | Version |
| http://localhost:8080/health/live | Liveness |

Sign in with the demo credentials, then explore: the admin dashboard (KPIs, 30-day charts, live event feed), users with switchable offset/cursor/keyset pagination, the jobs queue with live status updates over SSE, enrichment, reports, analytics, and the in-app Getting Started tour. Each page explains itself via the banner on top. Frontend details: [docs/guides/frontend.md](docs/guides/frontend.md).

The default run uses an in-memory repository so the app can be explored immediately. Set `APP_DATABASE_URL` and `APP_REDIS_ADDR` (see [.env.example](.env.example)) — or run `make docker-up` — for PostgreSQL (pgx + sqlc + startup migrations) and the Redis/asynq queue with a real worker process. Add `--profile observability` for Jaeger, Prometheus, and Grafana; see [docs/operations/observability.md](docs/operations/observability.md). ClickHouse analytics power the event charts when `APP_CLICKHOUSE_ADDR` is set; without it the app degrades to the in-memory event trail. For the enrichment streams, `make demo-up` adds Redpanda (Kafka API) and NATS with JetStream; the workers start consuming and the outbox relay starts publishing automatically.

## 🌍 Deploy to a VPS (one command)

A production-shaped, single-VPS stack — hardened base system, Nomad with mTLS+ACLs, automatic HTTPS (Caddy), Postgres/Redis, **your own CI + registry** (Woodpecker, tag-push builds and deploys), Prometheus/Grafana monitoring with the repo's alerts, and nightly backups:

```bash
make vps-setup     # once: guided questions -> generated secrets -> encrypted vault
make vps-deploy    # zero to https://your-domain in ~20 minutes (idempotent forever after)
```

Details: [docs/deployment/vps/QUICKSTART.md](docs/deployment/vps/QUICKSTART.md) · reference: [docs/deployment/vps/deployment.md](docs/deployment/vps/deployment.md) · ops: [docs/deployment/vps/runbook.md](docs/deployment/vps/runbook.md).

## 👤 Demo accounts

Seeded by the demo dataset (also shown as a one-click table on the login page when the `demo_mode` flag is on). Passwords are for local verification only.

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

Full flow to try: sign in as **u-0004**, file and submit an Expense ticket → **u-0002** approves the manager step → **u-0005** approves the final step → back as **u-0004**, deliver the result.

## 📚 Documentation

**Start here: [docs/README.md](docs/README.md)** — the documentation map: how everything is organized, a detailed reading order, tracks by role (backend / frontend / DevOps / student), and a find-by-task table.

The set is organized into seven categories:

| Category | What's inside |
|---|---|
| [getting-started](docs/getting-started/quickstart.md) | quickstart, prerequisites, daily development workflow, a guided study path, troubleshooting |
| [guides](docs/guides/application-overview.md) | one guide per feature — auth/MFA/tokens, RBAC, tickets, webhooks, streams, files, caching, frontend… |
| [reference](docs/reference/configuration.md) | every env var, every make target, CLI tools, API contract, project structure |
| [deployment](docs/deployment/README.md) | which-path decision guide + Docker, VPS (Ansible+Nomad), Kubernetes/Helm, GitOps, CI/CD |
| [operations](docs/operations/observability.md) | observability, SLOs, reliability, chaos lab, runbooks for all 24 alerts |
| [testing](docs/testing/README.md) | test strategy + the four traceability matrices |
| [architecture](docs/architecture/README.md) | the master set: 15 chapters (C4, flows, data model, security) + [26 ADRs](docs/architecture/decisions/README.md) |
| [roadmap](docs/roadmap/roadmap.md) | what's next, and the [known-issues registry](docs/roadmap/known-issues.md) with root-cause analysis |

## 🛠️ Development commands

```bash
make vet               # go vet
make lint              # golangci-lint
make test              # race-enabled tests
make coverage          # coverage report
make test-performance  # performance suite
make web-lint          # oxlint + oxfmt check for the SPA
make web-test          # vitest
make web-dev           # Vite dev server with API proxy
make web-build         # build web/dist for embedding
make seed-demo         # load the demo dataset
make docker-up         # full stack
```

## 📐 Project rules

- Start each feature with an OpenSpec change.
- Update OpenAPI for HTTP behavior.
- Update SQL migrations and sqlc queries for database behavior.
- Add tests and performance coverage for each user-facing or load-bearing feature.
- Update traceability matrices before a feature is considered complete.
