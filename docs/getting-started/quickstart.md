# Quickstart: zero to running app

This gets you from a fresh clone to a signed-in session against a live stack.
Everything below is local development — no cloud accounts, no external
services. Two ways to run:

- **Full stack** — `make docker-up`: the API, the worker, and all backing
  services (PostgreSQL, Redis, ClickHouse) in Docker, with the demo dataset
  pre-seeded. Best first experience.
- **Minimal** — `make run`: one Go process, zero dependencies, pure
  in-memory storage. Boots in seconds; good for a quick look at the code.

Need hot reload for development instead? Skip to
[development-workflow.md](development-workflow.md) (`make dev`).

## Prerequisites

Go 1.26 and Docker with Compose v2. Full details and per-task tooling:
[prerequisites.md](prerequisites.md).

## Option A — full stack in Docker

```bash
make docker-up
```

This builds the images and starts `api`, `worker`, `postgres` (16),
`redis` (7), and `clickhouse` (24.8). It runs in the foreground; `Ctrl-C`
stops it. `make docker-down` removes the containers *and* their volumes.

When the log settles, open **http://localhost:8080**. The first start
applies database migrations, seeds the demo login and the demo dataset
(compose sets `APP_DEMO_EMAIL`, `APP_DEMO_PASSWORD`, and `APP_DEMO_SEED=1`),
and the healthchecks gate startup — Postgres and Redis must report healthy
before the API starts.

Sign in with:

```
email:    demo@go-template.local
password: demo-password-123
```

The seed is idempotent — restarting never duplicates it. In production these
variables are simply not set, so no demo account exists.

## Option B — minimal in-memory run

```bash
make run            # go run ./cmd/api
```

No PostgreSQL, no Redis, no ClickHouse — the app boots on its in-memory
store and everything still works (registration, login, tickets, jobs). Data
is lost on restart by design; this mode exists so you can explore instantly.

Two things to know:

- **No demo account** is created: `demo@go-template.local` only exists when
  `APP_DEMO_EMAIL`/`APP_DEMO_PASSWORD` are set (compose sets them; a bare
  `make run` does not). Register your own user at `/register`, or export the
  variables before `make run`.
- **No SPA yet** if you have not built the web app: `web/dist` ships only a
  placeholder. Run `make web-install web-build` once to embed the real UI,
  or use the Vite dev server ([development-workflow.md](development-workflow.md)).

## Hybrid mode (the middle ground)

Infrastructure in containers, Go and Vite on the host with hot reload — the
daily-driver setup described in
[development-workflow.md](development-workflow.md). In short:

```bash
docker compose up -d postgres redis clickhouse

APP_DATABASE_URL="postgres://app:app@127.0.0.1:55432/app?sslmode=disable" \
APP_REDIS_ADDR="127.0.0.1:6379" \
APP_CLICKHOUSE_ADDR="127.0.0.1:9000" \
APP_CLICKHOUSE_USER=app APP_CLICKHOUSE_PASSWORD=app \
APP_DEMO_EMAIL="demo@go-template.local" \
APP_DEMO_PASSWORD="demo-password-123" \
APP_DEMO_SEED=1 \
APP_COOKIE_SECURE=0 \
go run ./cmd/api
```

Migrations are applied automatically at startup (advisory-locked, so
concurrent starts never race). Run `go run ./cmd/worker` in a second
terminal if you want the Redis-backed job queue processed.

## The demo dataset (`APP_DEMO_SEED=1`)

With `APP_DEMO_SEED=1` and a database configured, the API seeds a realistic
dataset — idempotently, skipped when already present:

- **500 users** created over 90 days; the first ten
  (`demo-u-0001@demo.example` … `demo-u-0010@demo.example`, password
  `password123`) can log in;
- **3 organizations** (Acme Corp, Globex, Initech) with members and roles;
- **120 tickets** spread across the whole approval lifecycle;
- **160 jobs** in every status, 40 reports, ~220 enrichment profiles;
- ~200 audit entries, 60 notifications (some unread — the bell is alive),
  2 API keys, an inactive webhook endpoint with delivery history;
- **90 days of events** (12 types, filled properties) powering the dashboard
  charts — stored in ClickHouse when configured, in the in-memory event
  trail otherwise.

Manage it by hand:

```bash
make seed-demo         # load the dataset if it is missing
make seed-demo-reset   # wipe everything demo-* and reseed
```

## Demo accounts

Seeded by the demo dataset and shown as a one-click table on the login page
when the `demo_mode` flag is on. Passwords are for local verification only.

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

One full flow to try: sign in as **u-0004**, file and submit an Expense
ticket → **u-0002** approves the manager step → **u-0005** approves the
final step → back as **u-0004**, deliver the result.

## What to explore first

| URL | What it is |
|---|---|
| `/` | The prerendered marketing landing (EN/RU/FR/DE) |
| `/dashboard` | The client workspace: KPIs, charts, the live event feed over SSE |
| `/tickets`, `/approvals` | The request workflow and the approver inbox |
| `/admin/*` | The back-office console: dashboard, users (offset/cursor/keyset pagination), jobs with live status, enrichment, reports, analytics, RBAC (`/admin/roles`, `/admin/policies`, `/admin/orgs`), audit log, settings & feature flags, webhooks |
| `/docs` | Scalar API reference rendered from the embedded OpenAPI spec |
| `/openapi.yaml` | The raw contract |
| `/metrics` | Prometheus metrics |
| `/version` | Build version/commit/date |
| `/health/live`, `/health/ready` | Liveness and dependency-checked readiness |
| `http://localhost:8025` | Mailpit (`--profile mail`) — read dev email such as password resets |
| `http://localhost:3000` | Grafana (`--profile observability`) — dashboards and alerts |

Each page explains itself via the banner on top, and the admin console has a
built-in **Getting Started** tour that walks the features in order.

Smoke-test the API directly:

```bash
curl -s -X POST localhost:8080/api/auth/login \
  -d '{"email":"demo@go-template.local","password":"demo-password-123"}'
```

## What runs with vs without each dependency

The app degrades, never dies (ADR 0004): a missing dependency removes one
capability, not the ability to boot.

| Dependency | Set by | Without it |
|---|---|---|
| PostgreSQL — `APP_DATABASE_URL` | compose, hybrid mode | In-memory store; everything works, data is lost on restart |
| Redis — `APP_REDIS_ADDR` | compose, hybrid mode | Jobs run in-process instead of the asynq queue; rate limits become per-process memory buckets |
| ClickHouse — `APP_CLICKHOUSE_ADDR` | compose, hybrid mode | Event charts fall back to the in-memory event trail |
| S3/MinIO — `APP_S3_BUCKET` | `--profile files` | Uploads land in an in-process blob store (lost on restart) |
| SMTP — `APP_SMTP_HOST` | `--profile mail` | Emails are logged to stdout instead of sent |
| Kafka/NATS — `APP_KAFKA_BROKERS`, `APP_NATS_URL` | `make demo-up` | Stream consumers and the outbox relay stay off (a startup warning notes it) |
| Tracing — `APP_OTLP_ENDPOINT` | `--profile observability` | Tracing disabled; logs and metrics unaffected |

The base `docker compose up` stack already includes Postgres, Redis, and
ClickHouse; the profiles add the optional ones:

```bash
docker compose --profile mail up -d            # + Mailpit
docker compose --profile files up -d           # + MinIO
docker compose --profile observability up -d   # + Jaeger, Prometheus, Grafana, Alertmanager, blackbox, Pyroscope
make demo-up                                   # + Redpanda (Kafka API) and NATS for the streams demos
```

## Ports

| Port | Service | Notes |
|---|---|---|
| 8080 | API + embedded SPA + SSE | The app |
| 55432 | PostgreSQL (host) → 5432 | Non-standard host port on purpose: 5432 collides with local port-forwarder quirks |
| 6379 | Redis | |
| 8123 / 9000 | ClickHouse HTTP / native | |
| 5173 | Vite dev server | Only during `make dev` / `make web-dev` |
| 1025 / 8025 | Mailpit SMTP / web UI | `--profile mail` |
| 19000 / 19001 | MinIO S3 API / console | `--profile files`; 9000 on the host already belongs to ClickHouse |
| 16686 / 4318 | Jaeger UI / OTLP HTTP | `--profile observability` |
| 9090 / 9093 / 9115 | Prometheus / Alertmanager / blackbox-exporter | `--profile observability` |
| 4040 | Pyroscope | `--profile observability` |
| 3000 | Grafana | `--profile observability` |
| 19092 | Redpanda (Kafka API, host listener) | `make demo-up` |
| 4222 / 8222 | NATS client / monitoring | `make demo-up` |

## Where next

- [prerequisites.md](prerequisites.md) — the exact toolchain, optional tools, and platform notes
- [development-workflow.md](development-workflow.md) — `make dev`, codegen, migrations, quality gates
- [study-path.md](study-path.md) — a guided reading order through the codebase
- [../reference/configuration.md](../reference/configuration.md) — every environment variable
- [../reference/make-targets.md](../reference/make-targets.md) — the full Makefile catalog
- [../deployment/README.md](../deployment/README.md) — from localhost to real deployments
- [../architecture/01-introduction-and-principles.md](../architecture/01-introduction-and-principles.md) — how it is designed and why
