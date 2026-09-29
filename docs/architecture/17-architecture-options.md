# 17 — Architecture Options

**In one sentence:** six candidate structures were evaluated against your
four goals — *no abstraction for its own sake, files ≤ 400 lines, feature
locality, scalability + supportability + conventionality* — and the
recommendation is **package-by-feature vertical slices** (option B): the
structure the Go community itself converged on, kept honest by the
contract tests you already have.

## 17.0 The goals, made measurable

| Goal | How it is measured here |
|---|---|
| **G1 · No abstraction for abstraction's sake** | Interface count is justified only by a *real second implementation* or a test seam; no layers whose only job is forwarding; no reflection/dispatch magic |
| **G2 · Laconic files (≤ 400 lines)** | File-size audit from [16 §16.3](16-current-architecture-assessment.md); target structure must make oversized files *unnatural* |
| **G3 · Feature locality** | "To add/modify feature X, I touch exactly these files" — count of shared hub files per feature |
| **G4 · Scalability, supportability, conventionality** | Scale path stays horizontal-replicas + workers; a newcomer from the Go mainstream finds the shape familiar |

Reference practice this evaluation follows (all recommend feature-first
Go structure with minimal interfaces):

- Ben Johnson, [Standard Package Layout](https://medium.com/@benbjohnson/standard-package-layout-7a7bb7d155f6) — domain types and logic in feature packages, details in subpackages, *interfaces defined where they are consumed*.
- Mat Ryer, [How I write HTTP services in Go after 13 years](https://matryer.com/posts/how-i-write-http-services-in-go-after-13-years/) — one thin `main`, per-feature router constructors, handlers take what they need, no globals.
- Simon Brown, [Modular monoliths — "package by feature"](https://simonbrown.je/modular-monolith/) — cohesion axis is the feature; the layer is a secondary dimension shown by *file names inside* the package.
- Three Dots Labs, [*Go With The Domain*](https://threedots.tech/go-with-the-domain/) — DDD *tactics* only where the domain pays for them.
- Go proverbs (Rob Pike): *"Clear is better than clever"*, *"A little copying is better than a little dependency"*; Dave Cheney, [Practical Go](https://dave.cheney.net/practical-go/presentations/qcon-china.html) — "interface pollution" as a smell.

## 17.1 Option A — keep the current layering, fix the warts

Keep `service` / `httpapp` / `repository` / `platform` as-is; fix only
W1–W3 from [16](16-current-architecture-assessment.md): move sentinel
errors out of `repository`, break the postgres→service import, split
oversized files.

```
internal/service/rbac.go → rbac_model.go + rbac_roles.go + rbac_http_mw.go   (≤400 each)
internal/service/tickets.go → tickets_state.go + tickets_approvals.go + tickets_types.go
```

- **Wins:** smallest diff; everything else untouched.
- **Loses:** does not deliver G3 at all — a feature still lives in five
  directories and three hub files; the hub files keep growing back
  (today's `router.go`/`ports.go`/`container.go` exist *because* of the
  axis, not because of carelessness).
- **Score:** simplicity 4 · locality 2 · supportability 3 · scale 3.

**Verdict:** a good *first phase*, not a destination.

## 17.2 Option B — package-by-feature vertical slices ★ recommended

Each feature owns one package containing its domain types, business
logic, HTTP handlers and (the interface for) its storage:

```
internal/
├── tickets/
│   ├── tickets.go        types + service entry (Ticket, list/get/create)   ≤400
│   ├── state.go          the state machine: actions, transitions, guards   ≤400
│   ├── approvals.go      chain steps, inbox, decide                        ≤400
│   ├── http.go           handlers + RegisterRoutes(gin)                    ≤400
│   └── store.go          type Store interface + errors (consumer-side)
├── tickets/store_memory.go / platform/postgres/tickets.go → implement tickets.Store
├── auth/ · users/ · orgs/ · webhooks/ · files/ · export/ · streams/ · … (same shape)
├── kernel/               shared domain vocabulary ONLY: ErrNotFound, ErrConflict,
│                         newID, Page/ListOptions — no feature logic
├── platform/             unchanged adapters (postgres+sqlc, queue, cache, mailer,
│                         kafka, nats, filestore, ratelimit, rbac, clickhouse, oauth)
├── httpx/                cross-cutting middleware + JSON/problem+json + ETag + SSE
└── bootstrap/            shrinks to: build platform adapters → call each feature's
                          New() → router registers each feature
```

Rules that keep G1 honest (this is where most Go projects over-rotate):

1. **Interfaces only where a second implementation is real.** Store
   interfaces stay (memory + Postgres are both real, contract-tested);
   everything else becomes concrete structs. Today's 23-interface
   `ports.go` dissolves into ~15 store interfaces living in their feature
   packages, defined *next to their consumers*, and 8 interfaces that
   exist only for tests/wiring die and become structs.
2. **No feature imports another feature's internals.** Cross-feature
   needs go through narrow exported functions or events (`tickets`
   notifying `notifications` uses the existing realtime/notification
   seam — which already exists as callbacks).
3. **File splitting follows seams, not arithmetic:** state machine,
   approvals, CRUD/list, http — the same seams the code already has
   internally.
4. **The layer axis does not disappear — it demotes to file names:**
   `state.go` / `http.go` / `store.go` *inside* the package. Simon
   Brown's point: the primary cohesion axis is the feature; the layer is
   readable at a glance within the folder.

- **Wins:** G3 fully — "add a feature = create `internal/<feature>/{feature.go, http.go, store.go}`, touch `bootstrap` + route registration (one line each)"; G2 becomes natural (each file has one seam); shared hubs shrink to trivial registration; contract tests keep proving memory ≡ Postgres.
- **Costs:** a mechanical, phased migration (roadmap in [18](18-migration-roadmap.md)); import-cycle care when two features reference each other's types (mitigated by `kernel` for shared vocabulary and by narrow interfaces at the consumer).
- **Score:** simplicity 4 · locality 5 · supportability 5 · scale 4.

**Verdict: recommended.** It is the mainstream Go answer to exactly your
four goals, it reuses the project's existing strengths (contract tests,
adapters, degradation), and it leaves a clean extraction path if a slice
ever needs to become a service.

## 17.3 Option C — strict Clean Architecture (add a use-case layer)

`cmd → adapters(http) → usecase → domain(entities) ← infrastructure`,
each layer with its own DTOs and mappers, dependency-inverted ports per
layer.

```
internal/
├── domain/tickets/{entity.go, repository.go, service.go}
├── usecase/tickets/{create.go, decide.go, list.go, dto.go, mapper.go}
├── adapter/http/tickets/handler.go     ← maps dto↔entity
└── infrastructure/persistence/postgres/tickets/repo.go
```

- **Wins:** maximal layer independence on paper; large orgs can split
  layer teams.
- **Loses against G1 head-on:** ~3× the files and 2 DTO/mapper layers per
  feature *now*, for a benefit (swapping Postgres, hosting the domain in
  another framework) the template will never cash in; Ben Johnson's and
  the Go community's standard critique — in Go this reads as ceremony.
  A CRUD feature grows from ~4 files to ~10.
- **Score:** simplicity 2 · locality 2 · supportability 3 · scale 3.

**Verdict: rejected** for this codebase's goals and size.

## 17.4 Option D — DDD tactical (aggregates, value objects, domain events)

Full tactical DDD: `Ticket` aggregate root enforcing invariants via
methods, value objects (`Email`, `TicketNumber`), domain events with a
dispatcher, repositories per aggregate, bounded contexts.

- **Honest assessment:** the *value* here is real but narrow — the two
  places with genuine invariants are the ticket state machine (already
  written as one, in `tickets.go` `ticketActions`) and stream application
  (already exactly-once in `streams.go`). The rest of the domain
  (users, files, settings, webhooks) is CRUD-with-permissions; DDD
  ceremony there buys nothing (Three Dots Labs' own guidance: tactics
  where complexity lives, not everywhere).
- **Cost:** new concepts (aggregates, events, VOs) on every review; more
  types per feature; risk of "DDD-speak" architecture-portfolio code —
  the opposite of G1.
- **Score:** simplicity 2 (wholesale) / 4 (selective) · locality 3 ·
  supportability 4-if-selective · scale 3.

**Verdict: rejected wholesale, adopted selectively** — inside option B's
Phase 4 the ticket state machine becomes a proper aggregate-ish type
(`state.go` methods own every transition) and stream apply stays a pure
function; nothing else changes.

## 17.5 Option E — vertical slices with a command bus (Mediatr-style)

The .NET-world "vertical slice" with `Command → Handler` registration and
pipeline behaviors, e.g. `Dispatch(CreateTicket{...})`.

- **Loses against G1:** the command bus is precisely abstraction-for-
  abstraction's-sake in Go — reflection or big type switches, hidden
  control flow, and the Go standard library has no Mediatr; community
  consensus (and Mat Ryer's patterns) is to call functions directly.
- **Verdict: rejected.** Option B already delivers vertical slices the
  Go-native way: the slice is the *package*, not a command record.

## 17.6 Option F — microservices

Extract tickets/auth/streams into separate deployables from day one.

- **Rejected for now,** by the standard monolith-first argument (ADR 0002):
  distributed transactions, eventual consistency and ops cost before
  product-market fit; the demo domain shares one Postgres schema with
  foreign keys. Option B keeps the *extraction seams* (feature owns its
  data contract via `store.go`; events already flow over brokers), so
  this door stays open without paying today.

## 17.7 Scorecard

Scale 1–5 per goal (5 = best fit for this codebase):

| Option | G1 no over-abstraction | G2 ≤400-line files | G3 feature locality | G4 supportability | G4 scale path | Conventional (Go mainstream) |
|---|---|---|---|---|---|---|
| A — layering + fixes | 4 | 3 | **2** | 3 | 3 | 4 |
| **B — package-by-feature** ★ | **4** | **5** | **5** | **5** | 4 | **5** |
| C — Clean Architecture | 2 | 2 | 2 | 3 | 3 | 2 (in Go circles) |
| D — DDD tactical (wholesale) | 2 | 3 | 3 | 4 | 3 | 3 |
| E — command-bus slices | 1 | 3 | 4 | 2 | 3 | 1 |
| F — microservices now | 3 | 5 | 4 | 2 | 5 | 3 (premature here) |

## 17.8 Recommendation

**Adopt B (package-by-feature), with D's tactics confined to the two
complex seams, and A's fixes as Phase 1 of the migration.** Concretely:
feature packages own their types/logic/http/store-interface; a tiny
`kernel` carries shared vocabulary; `platform/` adapters unchanged;
hub files reduced to registration lines. The full, step-by-step,
contract-test-guarded path is [18 — Migration Roadmap](18-migration-roadmap.md),
recorded as [ADR 0021](decisions/0021-package-by-feature.md).
