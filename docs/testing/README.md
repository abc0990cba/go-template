# Testing overview

This folder is the index for everything testing-related: the pyramid below,
the four traceability matrices, and the deep-dive docs. The Makefile is the
only entry point you need — every layer is a target, and each one is
documented in [../reference/make-targets.md](../reference/make-targets.md).

Quick start (from a clean checkout):

```bash
make test              # backend unit suite, race-enabled, no Docker needed
make test-integration  # spins up testcontainers (needs Docker)
make docker-up         # live stack, then:
make test-e2e          # backend e2e against it (skips itself if it is down)
cd web && npm ci && make web-test && make web-test-e2e
```

## The pyramid

| Layer | Command | What runs |
|---|---|---|
| Unit | `make test` | `go test -race ./...` — services, stores, middleware, reliability helpers |
| Integration | `make test-integration` | `go test -tags=integration ./test/integration/...` — real dependencies via testcontainers: Postgres, Redis, ClickHouse, Kafka, NATS, MinIO |
| Backend e2e | `make test-e2e` | `go test -tags=e2e ./test/e2e/...` against the live compose stack (happy path, RBAC, streams); skips itself when the stack is down |
| Frontend unit/integration | `make web-test` | vitest + Testing Library; route-level tests render the real TanStack tree with MSW mocks |
| Frontend e2e | `make web-test-e2e` | Playwright (Chromium) against the real Go binary with the embedded SPA: auth, tickets, the two-role approval chain, admin users, axe a11y gate, visual snapshots |
| Performance | `make test-performance` (+ `perf-login`, `perf-users`, `perf-analytics`, `perf-enrichment`) | k6 scenarios with p95 budgets |
| Fuzz | `make fuzz` | Go fuzzing on untrusted-input parsers: token verification, log email masking, problem+json, If-Match parsing; any crash corpus lands in `testdata/fuzz/` and replays in `make test` |
| Benchmarks | `make bench` | micro-benchmarks for hot paths (etag, problem+json, rate limiter); compare with benchstat |
| DAST | `make dast` | boots the compose stack and runs the OWASP ZAP baseline scan CI runs |
| Chaos | `make chaos-up` / `make chaos-*` | Toxiproxy fault injection for game days ([../operations/chaos.md](../operations/chaos.md)) |

Two gates worth knowing by name:

- **Store-contract suite** (`test/integration/store_contract_test.go` and
  friends): one scenario set that both the in-memory adapter and the real
  Postgres adapter must pass, so the dev-mode fake and production storage
  cannot drift. `streams_contract_test.go` does the same for the
  enrichment/inbox logic; `queue_test.go` drives the real asynq mux over
  Redis.
- **MSW contract gate** (`web/src/test/mocks/contract.test.ts`, part of
  `make web-test`): every MSW handler's path+method must exist in
  `api/openapi/openapi.yaml` — a drifted frontend mock fails CI exactly
  like backend codegen drift does.

Coverage floors: `make coverage` / `make coverage-check` (backend,
`COVERAGE_FLOOR`, run in CI's build-test job) and `make web-coverage`
(frontend thresholds in `vite.config.ts`). Frontend CI additionally gates
on `tsc --noEmit`, oxlint/oxfmt, `make web-check-boundaries`
(dependency-cruiser slice rules) and `make check-size-web` before any test
runs. The full walkthrough of each layer lives in
[testing.md](testing.md); the k6 scenarios and budgets in
[performance.md](performance.md); the manual pre-release pass in
[regression-checklist.md](regression-checklist.md).

## The four matrices

The matrix files in this folder answer "is everything we promised actually
built and tested?" from four angles. They are hand-maintained — the
discipline is the point, so update them in the same PR as the change they
describe:

| File | Shape | When to update |
|---|---|---|
| [requirements-matrix.md](requirements-matrix.md) | Requirement ID → feature → implementation status → interface (UI/API) | A capability ships, changes status (Implemented/Planned), or gains a new interface surface |
| [test-matrix.md](test-matrix.md) | Requirement × test layer (unit / integration / e2e) with the covering test files | You add, move, or retire a test that covers a requirement — including filling a "Planned" cell |
| [traceability-matrix.md](traceability-matrix.md) | Requirement → OpenSpec capability → code area → API/UI → tests → performance | A new requirement lands, or its spec, implementation area, API surface, or perf scenario moves — the full-chain view |
| [performance-matrix.md](performance-matrix.md) | k6 scenario → script → target → status, plus recorded runs per environment | A scenario or budget changes, or you record new p95/p99/error numbers |

Rule of thumb: requirements-matrix says **what exists**, test-matrix says
**how it is proven per layer**, traceability-matrix follows one requirement
**from spec to perf**, and performance-matrix tracks **the load-testing
budget**. New requirement IDs are minted in requirements-matrix first and
referenced by the other three.

## Reading order

1. [testing.md](testing.md) — the detailed description of every layer and gate.
2. [performance.md](performance.md) — how to run the k6 scenarios and what the budgets mean.
3. [regression-checklist.md](regression-checklist.md) — the manual pre-release pass.
4. [../reference/make-targets.md](../reference/make-targets.md) — every command this page mentions.
5. The matrices (above) — before claiming a feature is "done".
