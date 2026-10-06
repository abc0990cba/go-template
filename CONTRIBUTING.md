# Contributing

Thanks for working on this template. This file covers the day-to-day
mechanics of contributing; the documentation map in
[docs/README.md](docs/README.md) covers what everything is, and
[docs/getting-started/development-workflow.md](docs/getting-started/development-workflow.md)
covers the daily development loop in depth.

## Getting set up

```sh
git clone <your-fork>
cd go-template
make hooks        # install lefthook git hooks (gofmt + oxlint/oxfmt on commit)
make dev          # hot-reload dev loop (Go API + generated TS client + Vite)
# or, zero-setup:
make docker-up    # full stack in containers
```

Toolchain: Go 1.26, Node 22, Docker. Details and per-task extras:
[docs/getting-started/prerequisites.md](docs/getting-started/prerequisites.md).

## How changes flow

This repository is **OpenAPI-contract-first** and **OpenSpec-governed**:

1. **Contract first.** API changes start in `api/openapi/openapi.yaml`.
   Types for Go (`internal/api/api.gen.go`) and TypeScript
   (`web/src/shared/api/schema.gen.ts`) are *generated* — run
   `make generate` (and `npm run gen:api` in `web/`); CI fails on drift.
   Never hand-edit generated files.
2. **Significant features start as an OpenSpec change** — a proposal +
   specs + tasks under `openspec/changes/<name>/`. See
   [docs/reference/openspec.md](docs/reference/openspec.md). Small fixes
   don't need one.
3. **Database changes** are a golang-migrate pair in `migrations/postgres/`
   plus sqlc queries in `queries/`, then regenerate. Recipe:
   [docs/getting-started/development-workflow.md](docs/getting-started/development-workflow.md).
4. **Docs change with code.** If your change alters behavior, update the
   affected page under `docs/` — in the same PR. The three
   mirror-the-code docs (env vars, make targets, matrices) have a
   keep-them-honest rule: [docs/README.md § Conventions](docs/README.md#conventions).

## Quality gates

CI (`.github/workflows/ci.yml`) runs all of this; run the local equivalents
before pushing:

| Gate | Local command | Notes |
|---|---|---|
| Go formatting & vet | `make lint vet` | golangci-lint v2 |
| Go tests (race detector) | `make test` | |
| Integration tests | `make test-integration` | needs Docker (testcontainers) |
| Go coverage floor | `make coverage-check` | 46% default floor |
| File-size budgets | `make check-size` / `make check-size-web` | ≤ 400 lines/hand-written file |
| Vulnerabilities | `make vulncheck` | govulncheck |
| Codegen drift | `make generate` then `git diff --exit-code` | pinned tool versions |
| Frontend lint/format | `make web-lint` (oxlint/oxfmt) | |
| Frontend tests | `make web-test` / `make web-test-e2e` | Vitest + Playwright |
| Frontend boundaries | `make web-check-boundaries` | dependency-cruiser layering rules |

## Commit & PR conventions

Release notes are **generated from PR titles and labels**
(`.github/release-drafter.yml`), so prefix your PR title with its type:

- `feat:` — user-visible features (minor bump)
- `fix:` — bug fixes (patch bump)
- `perf:` / `reliability:` — performance & resilience work
- `deploy:` / `ci:` — deployment & pipeline changes
- `docs:` — documentation
- `breaking:` + `feat!:`/`fix!:` — breaking changes (major bump)

Keep PRs single-purpose; include tests for behavior changes; include a
screenshot for UI changes.

## Where things are

- Repo map: [docs/reference/project-structure.md](docs/reference/project-structure.md)
- Adding a feature end-to-end: [docs/architecture/15-template-guide.md](docs/architecture/15-template-guide.md)
- Debugging help: [docs/getting-started/troubleshooting.md](docs/getting-started/troubleshooting.md)
- Known sharp edges before you start: [docs/roadmap/known-issues.md](docs/roadmap/known-issues.md)
- Security issues: **do not open a public PR or issue** — see [SECURITY.md](SECURITY.md).

## License

By contributing, you agree that your contributions are licensed under the
[MIT License](LICENSE).
