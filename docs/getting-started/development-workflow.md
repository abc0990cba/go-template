# Development workflow: the daily loop

How you actually work in this repo day to day: run the dev loop, change the
contract, regenerate, keep the quality gates green, open a PR with a title
that the release tooling understands.

First run only: `make web-install` (npm ci for the SPA) — `make dev` and
every codegen step need `web/node_modules`.

## `make dev` — one terminal, three processes

```bash
make dev            # ./scripts/dev.sh
```

The script starts three processes and tears all of them down on `Ctrl-C`:

1. **The Go API with [air](https://github.com/air-verse/air) hot reload.**
   `.air.toml` builds `./cmd/api` into `tmp/api` and rebuilds whenever
   `.go`, `.yaml`, or `.sql` files change inside `internal/`, `cmd/`,
   `queries/`, or `migrations/` (300 ms debounce, stop-on-error). Without
   air installed it falls back to `go run ./cmd/api` — everything else still
   works, reloads just need a manual restart.
2. **The OpenAPI → TypeScript generator in watch mode**
   (`npm run gen:api:watch`): editing `api/openapi/openapi.yaml` rewrites
   `web/src/shared/api/schema.gen.ts`, so the typed client surfaces the
   change at compile time immediately.
3. **The Vite dev server on http://localhost:5173**, which proxies `/api`
   (with websockets), `/health`, `/docs`, `/openapi.yaml`, and `/version` to
   `http://localhost:8080`. The proxy sets `changeOrigin` and rewrites the
   `Origin` header to the API origin — the same-origin view the embedded SPA
   has in production, so session cookies just work and the backend's
   origin check passes.

Prefer separate terminals? Run the pieces yourself: `make run` (API),
`make web-dev` (Vite), `make worker` (background jobs — only useful once
Redis is configured).

## Two runtime modes — pick per task

| | In-memory quick run | Compose-assisted |
|---|---|---|
| Command | `make dev` or `make run` | `docker compose up -d postgres redis` (add `clickhouse` for charts), then `make dev` with `APP_DATABASE_URL` / `APP_REDIS_ADDR` exported (see [quickstart.md](quickstart.md)) |
| Storage | In-memory, lost on restart | PostgreSQL with real migrations; Redis-backed jobs |
| Migrations | None | Applied automatically at API boot |
| Demo dataset | No | Yes, with `APP_DEMO_SEED=1` |
| Use for | UI work, quick checks, reading the code | Anything touching SQL, queries, jobs, search, pagination, analytics |

The API picks its adapters purely from environment variables, so switching
modes is a restart, not a code change. Without `APP_DATABASE_URL` you get
the in-memory store; without `APP_REDIS_ADDR` jobs run in-process. The
Docker-based full stack (`make docker-up`) is for whole-stack verification,
not for editing — images rebuild on every change.

## Codegen: the contracts generate the code

Two contracts, one rule — you edit the contract, never the generated code:

- **HTTP:** `api/openapi/openapi.yaml` is the single source of truth for
  wire shapes and endpoints.
- **Data:** `queries/*.sql` plus the migration schema are the source for
  all SQL access.

```bash
make generate
```

runs all three generators (pinned versions — see
[prerequisites.md](prerequisites.md)):

| Generator | Input | Output |
|---|---|---|
| sqlc | `queries/*.sql` against the migration schema | `internal/platform/postgres/db/` (typed pgx/v5 queries) |
| oapi-codegen (types only) | `api/openapi/openapi.yaml` | `internal/api/api.gen.go` (request/response structs) |
| openapi-typescript | `api/openapi/openapi.yaml` | `web/src/shared/api/schema.gen.ts` (typed client shapes) |

**CI fails on drift.** The `codegen` job reruns `make generate`, lints the
contract (`make lint-spec`, vacuum rules in `api/openapi/.vacuum.json`), and
fails on `git diff --exit-code` — a PR that edits `queries/` or the spec
without regenerating cannot merge.

Frontend rule (enforced by review and [web/AGENTS.md](../../web/AGENTS.md)):
never hand-write wire shapes. Change the spec, run `npm run gen:api` (or let
`make dev` watch it), and fix whatever `tsc` surfaces at the call sites.

## Database changes

1. **Add a migration pair** in `migrations/postgres/` — golang-migrate
   naming: `NNNNNN_description.up.sql` and `NNNNNN_description.down.sql`,
   with `NNNNNN` the next sequential number (`000001_init` …
   `000023_identity_email_verified` today). Never edit an applied
   migration.
2. **Register the new `.up.sql`** in the `schema:` list of `sqlc.yaml` —
   sqlc does not glob, it wants the file listed.
3. **Add or adjust queries** in `queries/*.sql` (one file per domain).
4. **`make generate`** to regenerate the typed store.
5. Start the API — it **auto-migrates at boot** (golang-migrate, embedded
   files, Postgres advisory lock, so concurrent replicas and manual runs
   never race).

Manual control when you need it:

```bash
make migrate DATABASE_URL="postgres://app:app@localhost:55432/app?sslmode=disable"
make migrate-version DATABASE_URL=…     # current schema version
make migrate-force N=-1 DATABASE_URL=…  # recover a dirty migration
```

ClickHouse has its own small migration set in `migrations/clickhouse/`.

## Quality gates — what CI enforces

CI (`.github/workflows/ci.yml`) runs the same commands you run locally;
path filters skip the jobs a PR cannot affect.

Go side:

| Gate | Command |
|---|---|
| Build + vet | `go build ./... && go vet ./...` |
| Lint | `make lint` (golangci-lint v2.14.0, `.golangci.yml`) |
| Known vulnerabilities | `make vulncheck` (govulncheck) |
| **400-line budget** on hand-written production files | `make check-size` (excludes `*_test.go`, `*.gen.go`, generated `postgres/db`) |
| Coverage floor **46%** | `make coverage-check` (`COVERAGE_FLOOR=46`, race-enabled, generated code excluded) |
| Store contract tests | `make test-integration` (testcontainers) |

Web side:

| Gate | Command |
|---|---|
| Lint + format | `make web-lint` (oxlint + `oxfmt --check`) |
| Strict type-check + build | `cd web && npm run build` (`tsc --noEmit` first) |
| Architecture boundaries | `make web-check-boundaries` (dependency-cruiser, rules in `web/.dependency-cruiser.cjs`) |
| Size budgets | `make check-size-web` (≤400 lines per file, ≤80 per route) |
| Coverage ratchet | `cd web && npm run test:coverage` — thresholds in `vite.config.ts` (currently statements 42 / branches 38 / functions 35 / lines 42; ratchet up, never down) |

Cross-cutting:

- **Codegen drift** and contract lint (above) on every PR.
- **Browser E2E** (Playwright) when Go or web code changes.
- **Production image build** (`Dockerfile`).

House rules the linter and review enforce: configuration errors fail
startup instead of silently falling back; domain errors
(`repository.ErrNotFound`, `service.ValidationError`,
`service.ErrInvalidCredentials`, …) map to HTTP status codes in one place;
JSON is snake_case via struct tags; the frontend imports only across the
slice boundaries defined in [web/AGENTS.md](../../web/AGENTS.md).

## Git hooks

```bash
make hooks     # lefthook install (lefthook must be on PATH)
```

The pre-commit hook is deliberately fast — staged files only, no builds, no
tests (CI owns the deep checks):

- staged `*.go` → `gofmt -l -w` (fixed files are re-staged)
- staged `*.ts`/`*.tsx` → `oxlint`, then `oxfmt` (re-staged)

## Testing

```bash
make test               # go test -race ./... — the everyday gate
make test-integration   # store/stream contract tests; testcontainers → needs Docker; first run pulls images
make test-e2e           # happy-path journey against a live stack on :8080 (GO_TEMPLATE_BASE_URL to override)
make web-test           # vitest
make web-coverage       # vitest with the coverage ratchet
make web-test-e2e       # Playwright (installs Chromium on first run)
make bench              # micro-benchmarks for hot paths
```

The full strategy — matrix, fixtures, what to test where — lives in
[../testing/testing.md](../testing/testing.md).

## Branches and pull requests

Branch from `main`; CI runs on every PR. **PR titles do real work**:
release-drafter autolabels from title patterns, and the labels decide the
next release version. Use Conventional-Commit-style titles:

| Title pattern | Label | Version bump |
|---|---|---|
| starts with `feat` / `add` / `support` | feature | minor |
| starts with `fix` / `correct` / `repair` | fix | patch |
| starts with `docs` or mentions `readme`/`adr` | docs | patch |
| contains `!` or `breaking` | **breaking** | **major** |
| mentions metric/trace/monitor/grafana/prometheus/slo/alert/profil | observability | patch |
| mentions breaker/bulkhead/retry/chaos/retention/dlq/outbox | reliability | patch |
| starts with `ci` or mentions workflow/github actions | ci | patch |
| mentions helm/k8s/kubernetes/nomad/rollout/canary/deploy | deployment | patch |

Label anything that should stay out of the release notes `skip-changelog`.

Feature work starts with an OpenSpec change (`openspec/changes/<name>/` —
proposal, specs, design, tasks; validate with `make openspec-validate`).
The repo rules: update OpenAPI for HTTP behavior, migrations + sqlc queries
for database behavior, tests and performance coverage for load-bearing
features, and the traceability matrices before calling a feature done.
Details: [../reference/openspec.md](../reference/openspec.md) and
[CONTRIBUTING.md](../../CONTRIBUTING.md).

## Where next

- [troubleshooting.md](troubleshooting.md) — when the loop misbehaves
- [../guides/frontend.md](../guides/frontend.md) — the SPA stack in depth
- [../reference/configuration.md](../reference/configuration.md) — environment variables
- [../reference/make-targets.md](../reference/make-targets.md) — every make target explained
