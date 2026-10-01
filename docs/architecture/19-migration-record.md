# Migration Record: Package-by-Feature Restructure

**Date:** 2026-10-01 · **Scope:** `internal/` layout only — zero behavior change
· **Decision:** [ADR 0021](decisions/0021-package-by-feature.md) · **Plan:**
[18 — Migration Roadmap](18-migration-roadmap.md)

## Why

The codebase was well-layered but organized **by layer**: one feature was
scattered across five directories and plugged into three shared hub files.
Three goals drove the change:

1. **Feature locality** — "to build feature X, I take *this* folder" (one
   package owns a feature end to end).
2. **Laconic files** — no hand-written file over 400 lines (now enforced by
   `make check-size` in CI).
3. **No abstraction for its own sake** — interfaces exist only where a
   second implementation is real (the in-memory/Postgres store duality).
   This is the Go mainstream position (Ben Johnson's Standard Package
   Layout, Mat Ryer's HTTP-service patterns, Simon Brown's package-by-
   feature; sources in [17 — Architecture Options](17-architecture-options.md)).

## What changed, exactly (folder structure)

**Before** (package by layer):

```
internal/
├── service/      ALL business logic in one package (one file per feature)
│                 + ports.go (23 interfaces) + container.go + model refs
├── httpapp/      ALL HTTP handlers in one flat package (one file per feature)
├── model/        ALL domain structs of every feature in 2 shared files
├── repository/   in-memory store (one 6,000-line god-file)
└── platform/     adapters
```

**After** (package by feature):

```
internal/
├── <feature>/    × 19 — each feature owns everything about itself:
│                 types.go · <feature>.go (service) · store.go (interface)
│                 · http.go (RegisterHTTP) · tests — all ≤ 400 lines
│                 auth · users · rbac · tickets · streams · jobs · files ·
│                 export · webhooks · mfa · tokens · apikeys · email ·
│                 invitations · notifications · analytics · audit ·
│                 settings · search
├── kernel/       NEW — shared vocabulary only: sentinel errors, NewID,
│                 list options, idempotency record, Pinger
├── cache/        NEW — cache port + read-through helpers
├── httpx/        NEW — cross-cutting HTTP helpers (problem+json, error
│                 mapping, binding, session cookies); imports no feature
├── httpapp/      SLIMMED to assembly only: middleware chain, auth dispatch,
│                 guards, route registration, SPA, SSE snapshot, health
├── bootstrap/    + container.go (composition root; was service/container.go)
├── repository/   SPLIT: one memory_<feature>.go file per feature
├── platform/     unchanged adapters; now implement the feature Store
│                 interfaces (postgres, queue, cache, mailer, kafka, nats,
│                 clickhouse, filestore, ratelimit, rbac, oauth)
└── model/, service/   DELETED
```

## The moves, one line each

| Was | Now |
|---|---|
| `model.<Type>` (every feature mixed in 2 files) | the owning feature package (`tickets.Ticket`, `users.User`, …) |
| `service/<f>.go` (`XService`, `XRepo`) | `internal/<f>/` (`Service`, `Store` + types + logic) |
| `service/ports.go` (23 interfaces, one hub) | `internal/<f>/store.go` — interface next to its consumer; hub deleted |
| `httpapp/<f>.go` (App-method handlers) | `internal/<f>/http.go` (`RegisterHTTP(…)` + small `HTTP` deps struct) |
| `repository.ErrNotFound` (adapter!) | `kernel.ErrNotFound` (domain vocabulary) |
| `service/container.go` + `Deps` | `bootstrap/container.go` — one composition root for both binaries |
| `router.go` 409-line route table | ~20 one-line `RegisterHTTP` calls |
| monolithic `memory.go` / big postgres files | per-feature files in the same packages |

Dependency rules now (enforced by import direction + review):

```
httpapp → features → kernel/cache/httpx ← platform, repository
platform, repository → features (implement their Store interfaces)
feature → feature only via exported API; cycles forbidden
```

## How it was migrated (no big bang)

Five phases over one session, each keeping the tree compiling and every
test green: **(0)** guardrails (size budget + import rules in CI) →
**(1)** `kernel` extraction, fixing the adapter→domain error coupling →
**(2)** pilot carve-out (tickets: state machine split into its own file) →
**(3)** remaining 18 features moved one PR-sized step at a time, per-feature
recipe: create package → move types/service/handlers → repoint store
interface + adapters + router + wiring → move tests → build/vet/race gates →
**(4)** selective DDD tactics where the domain pays (tickets state machine,
streams apply). Oversized infra files were split along their seams
(demodata, devtools, postgres store, memory store, config, worker loops).

## What did NOT change

Public API (OpenAPI byte-identical), database schema, sqlc queries,
frontend, demo data, Docker/Helm/CI pipelines, generated code
(`api/api.gen.go`, `platform/postgres/db/`), and every runtime behavior —
graceful degradation, error mapping, security posture. Adapter packages kept
their implementations; only the interfaces they satisfy moved.

## Verification (final regression, this re-run)

| Gate | Result |
|---|---|
| `go build` + `go vet` (incl. integration/e2e tags) | clean |
| `go test -race ./internal/... ./cmd/...` | all packages ok |
| testcontainers integration suite (store contracts memory≡Postgres, exactly-once streams over real Kafka+NATS, queue, MinIO, ClickHouse) | ok (119 s) |
| `make check-size` (≤ 400 lines/file) | OK |
| web vitest | 7/7 |
| 33-check regression walkthrough, fully-configured instance | **PASS=33 FAIL=0** |
| walkthrough vs compose stack | PASS=27 — the 6 fails are JWT endpoints answering 501 because compose sets no `APP_JWT_SECRET` (by design) |
| e2e happy path (isolated api+worker+Postgres) | PASS |
| production compose image | builds, healthy |

Known, pre-existing (unchanged by this work): three timing-flaky tests that
pass on rerun; `TestTicketApprovalFlow`'s inbox premise fails identically on
a HEAD build (verified in a worktree); local golangci-lint binary predates
Go 1.26 (CI's pinned version is fine).
