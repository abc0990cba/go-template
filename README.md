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
- **i18n** — English/Russian UI toggle (EN shell + RU translations)
- **Rich demo data** — 500 users, 3 organizations, 90 days of events (12 types with property dimensions), audit entries, notifications, keys, and webhook history on first launch
- Gin HTTP API
- React admin SPA (Vite + TanStack Router/Query/Table, shadcn/ui, Tailwind v4) embedded in the binary, with Server-Sent Events for live updates
- OpenAPI contract
- OpenSpec workflow
- CI (GitHub Actions) with vet, golangci-lint, and race-enabled tests
- PostgreSQL persistence (pgx + sqlc + migrations) behind service ports
- Redis/asynq job queue with a real worker process and idempotent enqueueing
- Observability: structured logs, Prometheus metrics, OpenTelemetry traces
- PostgreSQL migrations and sqlc query layout
- Redis/asynq-ready background job design
- ClickHouse analytics schema
- Unit, integration, e2e, and performance test structure
- Reliability rules: graceful shutdown, context propagation, bounded concurrency, retry limits, and health checks

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

The default run uses an in-memory repository so the app can be explored immediately. Set `APP_DATABASE_URL` and `APP_REDIS_ADDR` (see `.env.example`) — or run `make docker-up` — for PostgreSQL (pgx + sqlc + startup migrations) and the Redis/asynq queue with a real worker process. Add `--profile observability` for Jaeger, Prometheus, and Grafana; see `docs/observability.md`. ClickHouse analytics power the event charts when `APP_CLICKHOUSE_ADDR` is set; without it the app degrades to the in-memory event trail.

## Project Rules

- Start each feature with an OpenSpec change.
- Update OpenAPI for HTTP behavior.
- Update SQL migrations and sqlc queries for database behavior.
- Add tests and performance coverage for each user-facing or load-bearing feature.
- Update traceability matrices before a feature is considered complete.
