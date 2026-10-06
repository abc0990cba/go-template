# Troubleshooting: local problems and fixes

Common failure modes on a developer machine, with the actual cause and fix
in this repo. Deployment-side problems have their own runbook — see
[Where to escalate](#where-to-escalate).

## Port already in use

**8080** (the app). Another process owns it — common with other dev servers.
For `make run`, move the API: `APP_HTTP_ADDR=:8081 make run`. Note the Vite
dev proxy targets `http://localhost:8080` (`apiTarget` in
`web/vite.config.ts`), so if you move the API, change that constant too or
skip Vite. For compose, override the port mapping (below).

**55432** (Postgres on the host). The compose file maps Postgres to host
port **55432** on purpose — plain 5432 is frequently occupied or shadowed by
local port-forwarder quirks (the comment lives right on the mapping in
`docker-compose.yml`). If 55432 is taken on your machine, do not "fix" it by
switching to 5432; pick another free port with an override file:

```yaml
# docker-compose.override.yml — gitignored, never commit it
services:
  postgres:
    ports: ["55433:5432"]
```

Then update `APP_DATABASE_URL` to match.

**6379** (Redis) and **8123/9000** (ClickHouse) — same override pattern if
you run local Redis/ClickHouse. Remember 9000 on the host already belongs to
ClickHouse in this stack, which is exactly why MinIO's S3 API is mapped to
**19000** instead.

**5173** (Vite). Another Vite project is running. One-off:
`cd web && npm run dev -- --port 5174`; permanent: `server.port` in
`web/vite.config.ts`.

Compose picks up `docker-compose.override.yml` automatically (it is in
`.gitignore` for exactly this), so local port changes never leak into
commits.

## `go:embed` errors when building the binary

`web/embed.go` declares `//go:embed all:dist`. The repo commits a
`web/dist/.gitkeep` placeholder so a fresh clone builds — but if `web/dist`
is missing entirely, compilation fails with
`pattern all:dist: no matching files found`. Fix:

```bash
make web-install web-build
```

Two related notes:

- Even with the placeholder present, `go build` succeeds but the binary
  serves no real UI — the SPA only exists after `make web-build`. The
  Dockerfile always builds the SPA first and copies `web/dist` in, so
  container images are unaffected.
- The embedded assets are baked in at compile time. After a `web-build`,
  restart the API (air rebuilds only on Go/SQL/YAML changes — `web/` is
  excluded from its watch scope).

## air / hot-reload issues

- **"air is not installed — falling back to 'go run'"**: `make dev` still
  works, but reloads need a manual restart. Install the pinned version:
  `go install github.com/air-verse/air@v1.67.4`.
- **A change did not trigger a rebuild**: check the watch scope in
  `.air.toml` — only `internal/`, `cmd/`, `queries/`, `migrations/` with
  `.go`/`.yaml`/`.sql` extensions. `web/`, `deploy/`, and `docs/` are
  deliberately excluded (Vite owns the frontend; the OpenAPI→TS watch owns
  the spec).
- **Rebuild loops or missed events** on Windows/WSL2: keep the repo inside
  the WSL filesystem, not `/mnt/c`.

## Docker compose issues

**"Mailpit/MinIO/Grafana is not running."** They are behind profiles and
intentionally off by default: `--profile mail` (Mailpit), `--profile files`
(MinIO), `--profile observability` (Jaeger, Prometheus, Grafana,
Alertmanager, blackbox-exporter, Pyroscope). Profiles can be combined:
`docker compose --profile mail --profile observability up -d`.

**MinIO image pull fails (quay.io auth or rate limit).** `minio/minio` was
removed from Docker Hub (2025); the compose file uses
`quay.io/minio/minio:latest`. If your network cannot reach quay cleanly,
either drop the `files` profile — the API degrades to its in-process file
store — or pin a mirror by overriding the image in your
`docker-compose.override.yml`.

**Image build stalls on a credential prompt** (mostly Docker Desktop
setups). `docker login` once, or skip image builds entirely and use the
hybrid mode — infrastructure via compose, the app via `go run`/`make dev`.
Both Docker Desktop and OrbStack work on macOS; OrbStack is also what the
`make vps-local` rehearsal uses.

## Nothing responds after `make docker-up`

The stack is sequenced by healthchecks: Postgres and Redis must pass
`pg_isready` / `redis-cli ping` before the API starts, and the API image has
its own `HEALTHCHECK` that runs `api -healthcheck` (a liveness probe of
itself). The very first start also builds two images and seeds the demo
dataset, so give it a minute. Then:

```bash
docker compose ps          # is api up, is it healthy?
docker compose logs -f api # what did it say last?
curl -f localhost:8080/health/ready
```

If `worker` shows as unhealthy-but-running, that is by design: its compose
entry disables the healthcheck (the shared image probes the api binary, and
the worker has no HTTP listener).

## Demo login rejected

`demo@go-template.local` / `demo-password-123` only exists when the API was
started with `APP_DEMO_EMAIL` and `APP_DEMO_PASSWORD` set. The compose stack
sets both; a bare `make run` does not. Either:

```bash
APP_DEMO_EMAIL=demo@go-template.local APP_DEMO_PASSWORD=demo-password-123 make run
# or just use the full stack:
make docker-up
# or register a fresh user at /register (works everywhere)
```

The richer demo dataset additionally requires `APP_DEMO_SEED=1` *and* a
configured `APP_DATABASE_URL` — seeding is skipped (idempotently) when the
marker user already exists, so restarts never duplicate it. Forcing a
reseed: `make seed-demo-reset`.

## API calls return 403 from the dev server

The backend rejects **state-changing** `/api` requests whose `Origin` header
does not match the request host (`originCheck` in
`internal/httpapp/security.go`) — that is the CSRF defense, and it returns
`403` problem+json ("cross-origin request rejected"). The Vite dev proxy
handles this by rewriting `Origin` to the API origin
(`web/vite.config.ts`), so pages served from `:5173` work through the
proxy. You get 403s only when you bypass the proxy — e.g. pointing the SPA
at `:8080` directly, or hand-crafting cross-origin POSTs. `GET`/`HEAD` are
exempt, and non-browser clients (curl, k6) send no `Origin` and pass.

## Integration tests fail

```bash
make test-integration   # go test -tags=integration ./test/integration/...
```

These use **testcontainers**: they need a working Docker daemon, and the
first run pulls PostgreSQL/Redis/ClickHouse/NATS images — slow once, fast
after. If everything fails with connection errors, check `docker ps` first;
on macOS also confirm the daemon is running and, on WSL2, that Docker
integrates with your distro.

## Playwright E2E issues

`make web-test-e2e` (run from the repo root; it changes into `web/` itself)
manages its own server unless you point it at a running stack:

| Variable | Default | Meaning |
|---|---|---|
| `E2E_BASE_URL` | *(unset)* | Set it to an already-running stack (e.g. `http://localhost:8080`) and the managed webServer is skipped entirely |
| `E2E_PORT` | `8090` | Port for the managed server |
| `E2E_ADMIN_EMAIL` | `e2e-admin@example.com` | Superadmin seeded on boot for admin journeys |
| `E2E_ADMIN_PASSWORD` | `e2e-admin-password-123` | Its password |

The managed server boots the API on the **in-memory** store with
`APP_DEMO_SEED=1`, and the first run pays for `npm run build` plus a Go
build — the webServer timeout is 300 s, so let it work. Specs share one
seeded server and run with a single worker on purpose (they create records
in it). Visual specs compare against committed snapshots; platform font
differences can fail them locally while CI passes.

The Go-side E2E suite (`make test-e2e`) expects a live stack on
`http://localhost:8080` — override with `GO_TEMPLATE_BASE_URL`.

## The Go E2E / regression hits the wrong stack

`make test-e2e` runs against whatever `GO_TEMPLATE_BASE_URL` points to
(default `http://localhost:8080`), and
`./scripts/regression-walkthrough.sh` takes its target from `BASE` (default
`http://localhost:18080`) — both sign in as the demo admin and **mutate
data** on that stack. If tests behave oddly, check you are not aiming them
at a long-lived compose stack full of demo data; a fresh `make run` (plus
the demo variables, see above) is a clean target.

## Still stuck

- Every make target with its behavior: [../reference/make-targets.md](../reference/make-targets.md)
- Every environment variable: [../reference/configuration.md](../reference/configuration.md)
- What runs in each container: [quickstart.md](quickstart.md) (ports and degradation tables)
- VPS-side problems (deploy, TLS, backups, Nomad jobs): [../deployment/vps/runbook.md](../deployment/vps/runbook.md)
- Observability while debugging (dashboards, alerts, traces): [../operations/observability.md](../operations/observability.md)
