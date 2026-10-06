# Prerequisites: the toolchain

## Required

| Tool | Version | Why | Check |
|---|---|---|---|
| Go | 1.26 | `go.mod` declares `go 1.26.0` with `toolchain go1.26.6`; with Go's automatic toolchain switching an older local Go downloads the pinned one, but installing 1.26+ avoids that | `go version` |
| Node.js | 22 | The SPA build and the generated API client (`.nvmrc` says `22`; CI and the Dockerfile use Node 22 too) | `node --version` |
| Docker + Compose | Compose v2 | The full stack, integration tests (testcontainers), image builds | `docker compose version` |
| make | any recent | Every workflow entry point (`make run`, `make dev`, `make docker-up`, …) | `make --version` |
| git | any recent | Hooks, CI, and your sanity | `git --version` |

npm ships with Node. Use your package manager or [nvm](https://github.com/nvm-sh/nvm)
(`nvm use` picks up `.nvmrc`).

Also useful, and installed almost everywhere already:

- **curl** — the API smoke tests and `scripts/regression-walkthrough.sh`
- **python3** — only that same regression script (it computes TOTP codes with it)
- **bash** — all repo scripts are POSIX `sh` or bash; both exist on macOS and any Linux

Compose v2 is the `docker compose` plugin (space, not hyphen). If `docker
compose version` fails but `docker-compose` works, you are on the legacy
standalone binary — upgrade; every command in these docs uses the plugin.

## What you do NOT need to install

Several tools a project like this usually demands are deliberately absent
from your machine's requirements:

- **sqlc, oapi-codegen, openapi-typescript, vacuum, govulncheck** — executed
  through pinned `go run` / `npx` invocations inside `make generate`, `make
  lint-spec`, `make vulncheck`, and the web npm scripts. The pinned version
  downloads on first use; you never install or upgrade them by hand.
- **golang-migrate CLI** — migrations are embedded in the binary and applied
  at API boot; `make migrate` runs the repo's own `cmd/migrate` through
  `go run`.
- **PostgreSQL, Redis, ClickHouse, Kafka, NATS** — all via Docker when a
  feature needs them ([quickstart.md](quickstart.md) shows which).

## Optional, per task

Everything here degrades gracefully — the relevant `make` target prints what
to install when it is missing.

| Task | Tool | Notes |
|---|---|---|
| Hot reload (`make dev`) | [air](https://github.com/air-verse/air) | Pinned: `go install github.com/air-verse/air@v1.67.4` (the same pin lives in `scripts/dev.sh` as `AIR_VERSION`). Without it, `make dev` falls back to plain `go run` — the API still starts, reloads just need a manual restart. |
| Lint Go (`make lint`) | [golangci-lint](https://golangci-lint.run) | CI pins **v2.14.0** (must be built with a Go ≥ the module's toolchain); config in `.golangci.yml`. `make lint` is a no-op with a hint when it is not installed. |
| Kubernetes demo (`make kind-up`) | `kind`, `kubectl`, `helm` | `scripts/kind-demo.sh` requires all three, then installs Prometheus (helm) and **Argo Rollouts v1.7.2** into the cluster itself for the canary walkthrough. |
| VPS deployment (`make vps-*`) | [Ansible](https://docs.ansible.com) | macOS: `brew install ansible`; Debian/Ubuntu: `sudo apt install ansible-core`. See [../deployment/vps/QUICKSTART.md](../deployment/vps/QUICKSTART.md). |
| Performance tests (`make test-performance`, `make perf-*`) | [k6](https://k6.io) | Scenarios live in `test/performance/`. |
| Spec workflow (`make openspec-validate`) | OpenSpec CLI | See [../reference/openspec.md](../reference/openspec.md); the target exits cleanly with a hint when absent. |
| Pre-commit hooks (`make hooks`) | [lefthook](https://lefthook.dev) | Must be on `PATH`; `make hooks` wires up `lefthook.yml`. |
| Browser E2E (`make web-test-e2e`) | Playwright Chromium | Installed automatically by the target (`npx playwright install chromium`). |

## Platform notes

- **macOS is the primary development platform.** Both Docker Desktop and
  [OrbStack](https://orbstack.dev) work; OrbStack is also what the local VPS
  rehearsal (`make vps-local`) uses for its throwaway Ubuntu machine. On
  Apple Silicon everything runs natively — the Dockerfile builds
  `CGO_ENABLED=0` static binaries, so arm64 images are first-class.
- **Linux** works out of the box — install Docker Engine + the Compose
  plugin from your distribution. `make` and `git` come from the package
  manager; air and lefthook install with `go install` / your Go bin dir on
  `PATH`.
- **Windows: use WSL2.** The repo ships no native Windows workflow: the dev
  scripts (`scripts/dev.sh`, `scripts/chaos.sh`, `scripts/kind-demo.sh`, the
  Ansible playbooks) are POSIX shell, and the Makefile assumes a Unix shell.
  Inside WSL2 (Ubuntu), follow the Linux instructions; `docker compose`
  works through Docker Desktop's WSL2 integration or a native engine in the
  distro. Clone the repository *inside* the WSL filesystem, not under
  `/mnt/c`, or file watching (air, Vite) becomes painfully slow.

## Version pinning policy

Generated code must be byte-identical on every machine, so the generators
are pinned, not floated:

- **Makefile pins** — sqlc `v1.31.1`, oapi-codegen `v2.8.0`, vacuum
  `v0.30.6`, govulncheck `v1.8.0`, written as `module@version` and executed
  with `go run` (see the `SQLC`/`OAPI_CODEGEN`/`VACUUM`/`GOVULNCHECK`
  variables at the top of the `Makefile`). You do not install these; `make
  generate` downloads the pinned version.
- **web pins** — `openapi-typescript@7.13.0` is pinned inside the
  `gen:api` scripts in `web/package.json`.
- **air** is pinned in `scripts/dev.sh`; **golangci-lint** in the CI
  workflow; **Go** by the `toolchain` directive; **Node** by `.nvmrc`.
- **Renovate** (`renovate.json`) keeps all of these current: weekly
  schedule, `rangeStrategy: pin`, and a regex manager that bumps the
  Makefile `module@version` pins as ordinary Go dependencies.

Do not bump generator versions by hand in a feature PR — let Renovate batch
them, because generator upgrades change output and belong to their own
change.

## Verify your setup

```bash
go version            # go1.26.x
node --version        # v22.x
npm --version
docker compose version
make --version
git --version

# Optional:
air -v                # v1.67.4 (only needed for make dev hot reload)
k6 version            # only for performance tests
kind version && kubectl version --client && helm version
ansible --version     # only for VPS deployment
```

Then prove the whole toolchain end to end:

```bash
make run              # boots the API on http://localhost:8080 (in-memory)
```

If that works, continue with [quickstart.md](quickstart.md).
