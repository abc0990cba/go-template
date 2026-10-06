# Makefile reference

The `Makefile` is the front door of the repository: every development loop,
quality gate, demo, and deployment task is a target. This page documents
every target and the variables they accept.

Everything here is a thin wrapper over a real command (a `go` invocation, a
`docker compose` profile, or a script under `scripts/` or
`deploy/ansible/scripts/`). When a table entry is not enough, read the
target itself — the recipes are deliberately short.

## Discovering targets

There is no `make help` target. To list every target:

```bash
grep '^[a-z-]*:' Makefile
```

or, for targets plus their `.PHONY` declarations:

```bash
make -qp 2>/dev/null | awk -F':' '/^[a-z]/ {print $1}' | sort -u
```

## Variables

Targets take overrides on the command line: `make seed-users USERS=50000`.
Command-line variables are exported to the recipe's environment, so Go
binaries that read env vars pick them up.

| Variable | Default | Used by |
|---|---|---|
| `VPS_SITE` | `prod` | All `vps-*` targets — set to `dev` or `local` to operate on another site |
| `TAG` | — (required) | `vps-app-deploy` — app image tag to deploy |
| `JOBS` | — (required) | `vps-update` — quoted list of Nomad jobs to re-render |
| `S` | — | `vps-logs` — container name fragment (`api`, `worker`, `postgres`, `caddy`, ...) |
| `FILE` | — (required) | `vps-restore` — backup dump filename |
| `DATABASE_URL` | `postgres://go_template:go_template@localhost:5432/go_template?sslmode=disable` | Documented for the `migrate` targets; note the binary itself reads `APP_DATABASE_URL` (see the migration table) |
| `APP_DATABASE_URL` | — | `migrate`, `migrate-version`, `migrate-force`, `seed-users`, `seed-demo` |
| `N` | — (required) | `migrate-force` — version to force (`-1` clears a dirty state) |
| `USERS` | `100000` | `seed-users` |
| `COVERAGE_FLOOR` | `46` | `coverage-check` — minimum merged coverage percentage |
| `MODE` | `offset` | `perf-users` — pagination scenario: `offset`, `cursor`, or `keyset` |
| `MS` | `2000` | `chaos-pg-latency` — injected latency per Postgres round-trip |
| `SEQ` | `1` | `demo-kafka-profile`, `demo-nats-contact` — stream sequence number |
| `COMPANY` | `Acme Corp` | `demo-kafka-profile` — company value in the profile event |

Version-pinned codegen and lint tools are plain Makefile variables
(`SQLC`, `OAPI_CODEGEN`, `VACUUM`, `GOVULNCHECK`); Renovate bumps each pin,
so every machine and CI run produce byte-identical output.

## Application and development

| Target | What it does | Prerequisites / notes |
|---|---|---|
| `run` | `go run ./cmd/api` — start the API server | Config via `APP_*` env vars (`.env.example`) |
| `dev` | One-terminal dev loop: `scripts/dev.sh` runs the API under `air` (falls back to `go run` without air), the OpenAPI→TS generator in watch mode, and the Vite dev server; Ctrl-C tears all three down | air (pinned via `AIR_VERSION` in the script), Node/npm; see [quickstart](../getting-started/quickstart.md) |
| `worker` | `go run ./cmd/worker` — start the background worker | Uses Redis/Asynq when `APP_REDIS_ADDR` is set, local polling otherwise |
| `hooks` | `lefthook install` — wire up the git hooks (`lefthook.yml`) | lefthook on PATH |

## Database migrations

The `migrate` targets run [cmd/migrate](cli-tools.md#cmdmigrate) outside the
app's boot path. The boot auto-migrate stays on as a fallback; golang-migrate
advisory-locks, so the two never race.

| Target | What it does | Prerequisites / notes |
|---|---|---|
| `migrate` | Apply pending migrations | Needs `APP_DATABASE_URL` in the environment: `make migrate APP_DATABASE_URL=postgres://...` (the `DATABASE_URL` Makefile variable is not read by the binary) |
| `migrate-version` | Print the schema version and dirty flag | Same env var |
| `migrate-force` | Force the version to `N` (`make migrate-force N=-1`) to recover a dirty state | Same env var; `N` is required |

## Codegen and contract

| Target | What it does | Prerequisites / notes |
|---|---|---|
| `generate` | Run sqlc, oapi-codegen, and the web `gen:api` script — regenerates all checked-in generated code | Tools run via `go run module@version` pins; compare with [OpenSpec](openspec.md) contract rules |
| `lint-spec` | Lint `api/openapi/openapi.yaml` with vacuum (operationIds present, JSON responses documented, problem+json errors declared) | Runs in CI's codegen job next to the drift gate |

## Quality

| Target | What it does | Prerequisites / notes |
|---|---|---|
| `lint` | `golangci-lint run ./...` | Prints install hint and no-ops if golangci-lint is missing; config in `.golangci.yml` |
| `vet` | `go vet ./...` | — |
| `vulncheck` | `govulncheck ./...` against the pinned version | — |
| `coverage` | Unit tests with `-race -covermode=atomic` over `./internal/...`, prints the total | — |
| `coverage-check` | Same suite with `-coverpkg=./internal/...` plus `./cmd/...`, then an awk pass dedupes blocks and enforces the floor | `COVERAGE_FLOOR=46` by default; generated files are excluded; fails below the floor |
| `check-size` | Fail if any hand-written production Go file exceeds 400 lines | Excludes `*_test.go`, `*.gen.go`, sqlc output |
| `check-size-web` | Fail if any hand-written web file exceeds 400 lines or any route file exceeds 80 | Excludes tests, generated files, and `shared/components/ui` |
| `bench` | Micro-benchmarks for hot paths (etag, problem+json, memory rate limiter) in `./internal/httpx/`, `./internal/platform/ratelimit/`, `./internal/httpapp/` | Compare runs with `benchstat` |
| `fuzz` | 30-second smoke pass over four fuzz targets (`FuzzVerifyAccess`, `FuzzMaskEmails`, `FuzzProblem`, `FuzzIfMatchParsing`) | Go runs one target per invocation; crashes land in `testdata/fuzz/` and replay in `make test` |

## Testing

| Target | What it does | Prerequisites / notes |
|---|---|---|
| `test` | `go test -race ./...` — the unit suite | See [testing](../testing/testing.md) |
| `test-integration` | `go test -tags=integration ./test/integration/...` | Tests start ephemeral Docker containers (Postgres, MinIO, ClickHouse) and skip when Docker or images are unavailable |
| `test-e2e` | `go test -tags=e2e ./test/e2e/... -count=1` | `-count=1` bypasses the test cache |
| `test-performance` | `k6 run test/performance/smoke.js` | k6 on PATH, otherwise prints an install hint; see [performance](../testing/performance.md) |
| `perf-login` | k6 login scenario | Needs a live stack with seeded users: `make docker-up && make seed-users` first |
| `perf-users` | k6 users-list scenario; `MODE=offset\|cursor\|keyset` picks the pagination comparison | Same prerequisite |
| `perf-analytics` | k6 analytics scenario | Same prerequisite |
| `perf-enrichment` | k6 enrichment-stream scenario | Same prerequisite plus the broker stack (`make demo-up`) |

## Seeding

| Target | What it does | Prerequisites / notes |
|---|---|---|
| `seed-users` | [cmd/seed](cli-tools.md#cmdseed): bulk-insert `USERS` perf users (default 100000) with working credentials | Requires `APP_DATABASE_URL` |
| `seed-demo` | [cmd/seed-demo](cli-tools.md#cmdseed-demo): load the demo dataset for the SPA | Requires `APP_DATABASE_URL` |
| `seed-demo-reset` | Same, with `-reset`: delete existing demo data first, then reseed | Same env var |

## Frontend

All `web-*` targets shell into `web/` npm scripts.

| Target | What it does | Prerequisites / notes |
|---|---|---|
| `web-install` | `npm ci` in `web/` | Run once (and after lockfile changes) |
| `web-dev` | Vite dev server | API on :8080 (or `make dev`, which starts both) |
| `web-build` | Production build | Output is embedded by the API |
| `web-lint` | ESLint plus Prettier format check | — |
| `web-test` | Vitest unit tests | — |
| `web-test-e2e` | Playwright tests (installs chromium first) | — |
| `web-coverage` | Vitest with coverage | — |
| `web-check-boundaries` | dependency-cruiser over `src/**` against the slice-architecture rules (`.dependency-cruiser.cjs`) | — |

## Docker

| Target | What it does | Prerequisites / notes |
|---|---|---|
| `docker-up` | `docker compose up --build` — the core stack (postgres, redis, api, worker, caddy, ...) | Docker; follow logs in the foreground |
| `docker-down` | `docker compose down -v` — stop and remove volumes | Destructive to container data |

## Enrichment streams demo

`demo-up` brings up the core stack plus Redpanda (Kafka API) and NATS with
JetStream; the other `demo-*` targets push events through the streams via
[cmd/devtools](cli-tools.md#cmddevtools). Full walkthrough:
[enrichment streams](../guides/enrichment-streams.md).

| Target | What it does | Prerequisites / notes |
|---|---|---|
| `demo-up` | Start the core + brokers compose profile, print the demo login and suggested next targets | Docker |
| `demo-down` | Tear the demo stack down, removing volumes | — |
| `demo-kafka-profile` | Publish one profile event for `demo-u-0001` to Kafka; the user's page gains the "Profile (Kafka)" card | `demo-up` running; `SEQ`, `COMPANY` override |
| `demo-nats-contact` | Publish one contact event to NATS JetStream; the "Contact (NATS)" card | `demo-up` running; `SEQ` override |
| `demo-enrichment` | End to end: publish both streams for one user, then poll the API for the merge | `demo-up` running |
| `demo-dlq` | Publish a poison message, watch it land in dead letters, then replay it | `demo-up` running |
| `demo-outbox` | Consume `user-events.v1` and print the `user.enriched` event the outbox relay published | `demo-up` running; 15s timeout |
| `demo-breaker` | Trip the enrichment circuit breaker against a flaky upstream, then watch it recover | `demo-up` running; watch `app_cb_state` on `/metrics` |
| `demo-ratelimit` | Hammer the API until 429s come back with `Retry-After` | `demo-up` running |
| `demo-idempotency` | `scripts/demo-idempotency.sh`: POST the same ticket twice with one `Idempotency-Key`; the retry replays | `demo-up` running, demo data seeded, `jq` on PATH |

## Chaos lab

Fault injection through Toxiproxy: with the chaos profile up, the app talks
to postgres/redis through proxies and you break the middle. Game-day
scenarios: [chaos](../operations/chaos.md). The `chaos.sh` commands talk to
the Toxiproxy API at `http://localhost:8474` (override with `TOXIPROXY_API`).

| Target | What it does | Prerequisites / notes |
|---|---|---|
| `chaos-up` | Start the compose `--profile chaos` stack (proxies in front of postgres and redis) | Docker |
| `chaos-down` | Stop the chaos profile | — |
| `chaos-status` | List proxies and active toxics | `chaos-up` |
| `chaos-pg-latency` | Add `MS` ms latency (default 2000, jitter 50) to every Postgres round-trip | `chaos-up` |
| `chaos-pg-down` | Cut the Postgres proxy — timeouts, not refusals | `chaos-up` |
| `chaos-redis-down` | Cut the Redis proxy | `chaos-up` |
| `chaos-restore` | Remove all toxics and restore both proxies | — |

Best experienced with the observability profile up so dashboards and alerts
react; see [observability](../operations/observability.md).

## Progressive delivery and orchestration

Primary path: a Kubernetes canary with metric-gated auto-rollback (Argo
Rollouts), walked through locally on kind. Lighter alternative: Nomad's
native canary + auto-revert (`deploy/nomad/`).

| Target | What it does | Prerequisites / notes |
|---|---|---|
| `kind-up` | `scripts/kind-demo.sh up` — kind cluster, build + load the image, Prometheus, Argo Rollouts, and the app installed as a Rollout (canary + analysis, in-memory store) | docker, kind, kubectl, helm; idempotent (skips done steps) |
| `kind-release` | `scripts/kind-demo.sh release` — push a new revision and watch the canary gate (20% → analysis → 50% → analysis → 100%) | `kind-up` done |
| `kind-down` | Delete the kind cluster | — |
| `nomad-up` | Start a Nomad dev agent in the background (log `/tmp/nomad-dev.log`, UI http://localhost:4646) | nomad on PATH |
| `nomad-validate` | Validate `deploy/nomad/go-template.nomad.hcl` | — |
| `nomad-run` | Run the Nomad job | `nomad-up` |
| `nomad-stop` | Kill the dev agent | — |
| `helm-lint` | `helm lint deploy/helm/go-template` | helm |
| `helm-template` | Render the chart to stdout | helm |

## Security

| Target | What it does | Prerequisites / notes |
|---|---|---|
| `dast` | Boots the compose stack, waits for `/health/ready`, runs the same OWASP ZAP baseline scan CI runs against http://127.0.0.1:8080, writes `zap/zap-report.html`, tears the stack down | Docker; report in `zap/` (gitignored) |

## OpenSpec

| Target | What it does | Prerequisites / notes |
|---|---|---|
| `openspec-validate` | `openspec validate` | openspec CLI on PATH, otherwise prints a pointer to [the OpenSpec page](openspec.md) |

## VPS deployment

One VPS, one command: `make vps-setup` once, then `make vps-deploy` forever.
All targets accept `VPS_SITE` (default `prod`) to operate on another site,
e.g. `VPS_SITE=dev make vps-status`. Under the hood these run
`deploy/ansible/scripts/*.sh`; read [the VPS quickstart](../deployment/vps/QUICKSTART.md)
for the full flow.

| Target | What it does | Prerequisites / notes |
|---|---|---|
| `vps-setup` | Guided first-time setup: asks the six questions, generates and vault-encrypts every secret, writes the inventory | Run once per site; refuses if secrets already exist |
| `vps-doctor` | Verify workstation prerequisites, SSH reachability, DNS A-records, and VPS sizing | Read-only, fixes nothing |
| `vps-deploy` | THE one command: full-stack Ansible converge on the VPS (idempotent) | After `vps-setup` |
| `vps-check` | Dry-run the full deploy (`--check --diff`) — no changes made | — |
| `vps-app-deploy` | Deploy an app image tag: `make vps-app-deploy TAG=v1.2.3` | `TAG` required |
| `vps-update` | Re-render and redeploy selected Nomad jobs: `make vps-update JOBS="caddy monitoring"` | `JOBS` required |
| `vps-rollback` | Revert the app to the previous successful Nomad deployment (no image rebuild) | — |
| `vps-status` | Nomad node + job status on the VPS | — |
| `vps-logs` | Follow a container's logs: `make vps-logs S=api` (last 200 lines, `-f`) | `S` selects the container by name fragment |
| `vps-ui` | Open SSH tunnels to loopback-only admin surfaces: Nomad UI :4646, Grafana :3001, Uptime Kuma :3002, Asynqmon :8082, Loki :3100 | Keep the terminal open; Ctrl-C closes |
| `vps-backup` | Trigger the backup service now and list recent archives | — |
| `vps-restore` | Restore a dump: `make vps-restore FILE=go_template-YYYYMMDD-HHMMSS.dump` | `FILE` required; destructive to the target database — runbook: [restore](../deployment/vps/runbook.md) |
| `vps-acme-production` | Flip `acme_staging` to false in the site inventory and redeploy Caddy against the production Let's Encrypt CA | Run only after staging issuance succeeded |
| `vps-local` | Rehearse the ENTIRE deploy on a local OrbStack Ubuntu machine, then verify with a PASS/FAIL table | OrbStack; first run ~10-20 min; see [local rehearsal](../deployment/vps/LOCAL-REHEARSAL.md) |
| `vps-verify` | Post-deploy verification table (Nomad jobs over SSH, public HTTPS via `curl --resolve`); exits non-zero on failure | Works for prod, dev, and local sites |

## Common workflows

### First run

```bash
make hooks                 # git hooks
make web-install           # npm ci
cp .env.example .env       # then edit secrets
make docker-up             # core stack (or: make demo-up for the broker stack)
make migrate APP_DATABASE_URL=postgres://go_template:go_template@localhost:5432/go_template?sslmode=disable
make seed-demo             # realistic dataset for the SPA
make dev                   # API (hot reload) + TS codegen + Vite in one terminal
```

The quickstart in [getting-started/quickstart.md](../getting-started/quickstart.md)
walks the same path with explanations.

### Daily development loop

```bash
make dev                   # terminal 1
make test                  # fast unit gate, race detector on
make lint && make vet      # before you commit
```

### Pre-PR checklist

What CI runs, in one paste:

```bash
make generate && git diff --exit-code   # codegen drift gate
make lint-spec                           # OpenAPI contract lint
make lint && make vet && make vulncheck
make test
make coverage-check                      # enforces the coverage floor
make check-size && make check-size-web   # file size budgets
make web-lint && make web-test && make web-build
```

### Running one integration test

```bash
go test -tags=integration ./test/integration/... -run '^TestNameHere$' -count=1 -v
```

The suite provisions its own throwaway containers; anything that cannot get
a container skips itself rather than failing.

### Performance session

```bash
make docker-up             # or make demo-up for the enrichment scenarios
make seed-users USERS=100000
make perf-login
MODE=keyset make perf-users
```

Scenario inventory: [performance](../testing/performance.md).

### Releasing

Local canary rehearsal (Argo Rollouts on kind):

```bash
make kind-up
make kind-release          # watch the metric gate pass — or force a rollback
make kind-down
```

VPS production:

```bash
make vps-check                       # dry-run
make vps-deploy                      # full stack
make vps-app-deploy TAG=v1.2.3       # app image only
make vps-verify                      # PASS/FAIL table
make vps-rollback                    # if anything looks wrong
```
