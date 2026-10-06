# Docker & Compose reference

The `Dockerfile` builds one image that carries everything; `docker-compose.yml`
orchestrates it locally with opt-in profiles and overlays. This page is the
reference for both. For the pick-a-path overview, see [README.md](README.md);
day-to-day dev workflows live in `../getting-started/quickstart.md`.

## The image (`Dockerfile`)

Three stages, from source to a shell-less runtime:

| Stage | Base | What happens |
|---|---|---|
| 1. `web` | `node:22-alpine` | `npm ci` + `npm run build` → React SPA static assets in `/web/dist` |
| 2. `build` | `golang:1.26-alpine` | `CGO_ENABLED=0` static build. **Copies `/web/dist` in first** — the SPA is embedded via `go:embed`, so it must exist before `go build`. Stamps `VERSION`/`COMMIT`/`DATE` build args through `-ldflags "-X main.version=… …"`. Produces `/out/api` **and** `/out/worker` — both binaries ship in one image. |
| 3. runtime | `gcr.io/distroless/static-debian12:nonroot` | Binaries + `migrations/` only. `USER nonroot`, `EXPOSE 8080`, `APP_HTTP_ADDR=:8080`. |

The runtime stage has **no shell, no package manager, no curl**. That is why
the `HEALTHCHECK` is a binary flag, not a shell snippet:

```dockerfile
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD ["/app/api", "-healthcheck"]
```

`api -healthcheck` GETs `/health/live` on the local port and exits non-zero on
failure (see `cmd/api/main.go`). `CMD ["/app/api"]` is the default; compose
and every orchestrator override it to `/app/worker` for the worker.

Build it standalone:

```bash
docker build --build-arg VERSION=v1.2.3 --build-arg COMMIT="$(git rev-parse --short HEAD)" -t go-template:local .
```

## Base stack (`docker-compose.yml`)

`make docker-up` (=`docker compose up --build`) starts all of this; the api
serves the embedded SPA at http://localhost:8080 with the demo dataset
seeded (`APP_DEMO_SEED=1`, login `demo@go-template.local` /
`demo-password-123`).

| Service | Image | Host port | Notes |
|---|---|---|---|
| `api` | built from `Dockerfile` | `8080` | Env: `APP_DATABASE_URL` → `postgres:5432`, `APP_REDIS_ADDR` → `redis:6379`, `APP_CLICKHOUSE_ADDR` → `clickhouse:9000`, `APP_OTLP_ENDPOINT` → `jaeger:4318`, `APP_COOKIE_SECURE=0`. `depends_on` postgres/redis with `condition: service_healthy`. |
| `worker` | same image, `/app/worker` | — (metrics `:9090` in-network) | `APP_REPORT_CRON=0 3 * * *`, `APP_METRICS_ADDR=:9090` for the stream/outbox/DLQ gauges. **Healthcheck explicitly disabled**: the shared image's check probes the `api` binary, and the worker has no HTTP listener — disabled rather than lying unhealthy. |
| `postgres` | `postgres:16` | **`55432`** → 5432 | The host port deliberately differs from 5432 to dodge local port-forwarder quirks (a host Postgres or SSH tunnel on 5432 would collide). `pg_isready` healthcheck; volume `pgdata`. |
| `redis` | `redis:7-alpine` | `6379` | `redis-cli ping` healthcheck. |
| `clickhouse` | `clickhouse/clickhouse-server:24.8-alpine` | `8123` (HTTP), `9000` (native) | DB/user/pass `app`. Volume `clickdata`. |

Compose defaults are dev-shaped on purpose: `sslmode=disable`, JSON logs,
demo seed. Do not copy them to production; the chart and the Nomad job carry
the production env shape.

## Profiles (same file, opt-in)

Start a profile with `docker compose --profile <name> up -d` (or add it to
`COMPOSE_PROFILES`).

### `mail` — Mailpit

| Service | Host ports | Purpose |
|---|---|---|
| `mailpit` (`axllent/mailpit:v1.24`) | `8025` (web UI), `1025` (SMTP) | Read dev email (password resets, invites). api/worker already default `APP_SMTP_PORT=1025`. |

### `files` — MinIO (S3)

| Service | Host ports | Purpose |
|---|---|---|
| `minio` (`quay.io/minio/minio`) | **`19000`** (S3 API), `19001` (console) | Object storage for the files feature. **The S3 API is mapped to host `19000`, not `9000`, because ClickHouse already owns `9000`.** |

Two portability notes baked into the file:

- The image is `quay.io/minio/minio` — `minio/minio` was removed from Docker
  Hub in 2025 and quay.io is the publisher now.
- If quay requires auth on your network, drop the `files` profile entirely:
  the api degrades to its in-process file store.

### `observability` — the full telemetry stack

| Service | Image | Host port | Purpose |
|---|---|---|---|
| `jaeger` | `jaegertracing/all-in-one:1.63.0` | `16686` (UI), `4318` (OTLP HTTP) | Traces; the api/worker `APP_OTLP_ENDPOINT=jaeger:4318` points here |
| `pyroscope` | `grafana/pyroscope:1.8.0` | `4040` | Continuous profiling. **Opt-in per process**: set `APP_PYROSCOPE_URL=pyroscope:4040` (see `.env.example`) so base stacks never open background upload connections |
| `prometheus` | `prom/prometheus:v2.54.1` | `9090` | Mounts `deploy/prometheus.yml` and `deploy/prometheus/rules/` (alert + SLO rules) read-only; scrapes the api's `/metrics` and the worker's `:9090` |
| `alertmanager` | `prom/alertmanager:v0.27.0` | `9093` | Fired/silenced alerts UI; config from `deploy/alertmanager/` |
| `blackbox-exporter` | `prom/blackbox-exporter:v0.25.0` | `9115` | Synthetic "is the front door actually open" probes — the check whitebox `/metrics` cannot answer. Prometheus scrapes the exporter, not the app |
| `grafana` | `grafana/grafana:11.2.0` | `3000` | Dashboards provisioned from `deploy/grafana/`; anonymous Admin auth for local use |

Dashboards, alerts, and SLO rules are documented in
`../operations/observability.md` and `../operations/slo.md`.

## Overlays (separate `-f` files)

Overlays merge on top of the base file:

```bash
docker compose -f docker-compose.yml -f <overlay>.yml up -d
```

### `docker-compose.brokers.yml` — Redpanda + NATS (`make demo-up`)

The enrichment-streams demo stack. `make demo-up` wraps exactly the command
above; `make demo-down` tears it down with volumes.

- `redpanda` (`docker.redpanda.com/redpandadata/redpanda:v24.2.7`): Kafka
  protocol without ZooKeeper/KRaft gymnastics. Two listeners —
  `INTERNAL://redpanda:9092` for containers,
  `EXTERNAL://localhost:19092` advertised so host-side tools (`cmd/devtools`,
  kcat) reach the cluster without DNS work.
- `nats` (`nats:2.10-alpine`, `-js` = JetStream): `4222` client, `8222`
  monitoring.
- The overlay injects `APP_KAFKA_BROKERS=redpanda:9092` and
  `APP_NATS_URL=nats://nats:4222` into api and worker, and publishes the
  worker metrics port on the host as `19090` (`:9090` would collide with
  Prometheus).

Then drive it with `make demo-enrichment`, `make demo-dlq`, `make
demo-outbox`, `make demo-breaker`, `make demo-ratelimit`,
`make demo-idempotency` (see `../reference/make-targets.md` and
`docs/guides/enrichment-streams.md`).

### `docker-compose.chaos.yml` — toxiproxy fault injection (`make chaos-up`)

The chaos lab (`../operations/chaos.md`). Adds `toxiproxy`
(`ghcr.io/shopify/toxiproxy:2.12.0`, control API/UI on `8474`) plus a one-shot
`toxiproxy-setup` that registers two proxies: `pg` (`postgres-proxy:5432` →
`postgres:5432`) and `redis` (`redis-proxy:6379` → `redis:6379`). The api and
worker are repointed at the proxy aliases, so fault injection happens
transparently; `make chaos-pg-latency MS=2000`, `make chaos-pg-down`,
`make chaos-redis-down` inject, `make chaos-restore` heals. Best experienced
with the observability profile up so dashboards and alerts react.

### `docker-compose.smoke.yml` — committed port override

Re-maps the api to host `18080` for runs where `8080` is busy — the
regression script's default target is `:18080`.

### `docker-compose.override.yml` — yours, gitignored

Docker Compose auto-merges this file when present; it is in `.gitignore`, so
commit nothing to it. Typical use (as on the maintainer's machine): move the
api to `18080`, Grafana to `13000`, and set a dev-only `APP_JWT_SECRET` to
enable the JWT grant endpoints locally. The `!override` tag replaces the
base port list instead of appending to it.

## Common operations

```bash
# Rebuild after a code change (Go or web — the SPA is baked into the image):
docker compose up --build api          # or just: make docker-up
docker compose build                   # image only

# Logs:
docker compose logs -f api
docker compose logs -f worker

# One service + its dependencies:
docker compose up -d --wait postgres redis clickhouse

# Reset everything (named volumes pgdata / clickdata / filedata go too):
docker compose down -v                 # = make docker-down

# Enter Postgres from the host (note the 55432):
psql -h localhost -p 55432 -U app -d app

# Exec into a container — distroless has no shell, so target the infra:
docker compose exec postgres psql -U app -d app
docker compose exec redis redis-cli
```

## Port map (everything, at a glance)

| Host port | Service | Why this port |
|---|---|---|
| `8080` (`18080` with overrides/smoke) | api (SPA + API + SSE) | app default |
| `55432` | Postgres | dodges a local 5432 |
| `6379` | Redis | default |
| `8123` / `9000` | ClickHouse HTTP / native | defaults |
| `19000` / `19001` | MinIO S3 API / console | `9000` collides with ClickHouse |
| `8025` / `1025` | Mailpit UI / SMTP | defaults |
| `16686` / `4318` | Jaeger UI / OTLP HTTP | defaults |
| `4040` | Pyroscope | default |
| `9090` / `9093` / `9115` | Prometheus / Alertmanager / blackbox-exporter | defaults |
| `3000` (`13000` with override) | Grafana | default |
| `19092` / `4222` / `8222` | Redpanda EXTERNAL listener / NATS client / NATS monitoring | EXTERNAL is advertised to host tools as `localhost:19092` (containers use `redpanda:9092` in-network); NATS defaults |
| `19090` | worker metrics (brokers overlay) | `9090` is taken by Prometheus |
| `8474` | toxiproxy control API/UI | default |
