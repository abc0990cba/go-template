# 18 — Migration Roadmap: to Package-by-Feature

> **Status: COMPLETE (2026-10-01).** All five phases landed in a single
> session: guardrails in CI (`make check-size`), the kernel extraction, the
> tickets pilot, all 18 remaining features rolled over, and the optional
> DDD tactics where they pay (tickets state machine, streams apply).
> Verification at the end: `go vet` clean (incl. integration/e2e tags),
> `go test -race` green, full testcontainers integration suite green, the
> production compose image healthy, and the 33-check regression walkthrough
> `PASS=33 FAIL=0` against the refactored code. The e2e
> `TestTicketApprovalFlow` failure is pre-existing (reproduced on a HEAD
> worktree — the approvals-inbox premise of the test, not the code).

**In one sentence:** five incremental phases move the codebase from
package-by-layer to package-by-feature (the [recommended option
B](17-architecture-options.md)) — each phase is small, keeps `make test`
and the contract tests green, and ends with a "feature playbook" that
makes adding a feature a matter of creating three files.

**The two iron rules for every phase:**

1. **No big bang.** One feature (or one concern) per PR; the tree
   compiles and all tests pass at every commit.
2. **The contract tests are the referee.**
   `test/integration/store_contract_test.go` already proves memory ≡
   Postgres; if a phase cannot keep them green, the phase is wrong.

## Phase 0 — Guardrails (½ day)

Make the new rules enforceable before moving anything:

| Guardrail | Implementation |
|---|---|
| **≤ 400 lines per hand-written file** | CI script (or `Makefile` target run in CI): `find internal cmd -name '*.go' ! -name '*.gen.go' ! -path '*/db/*'` piped through an awk one-liner failing on files over 400. Generated code (`api/api.gen.go`, `platform/postgres/db/`) exempt. |
| **Import direction** | `golangci-lint` `depguard` (or a small test): `internal/<feature>` may not import another feature package or any `platform/*` except through its declared store interface; `kernel` imports nothing internal. |
| **Baseline docs** | Record the budget + rules in [15 — Template Guide](15-template-guide.md) ("Project rules" section). |

Exit: CI fails on a 401-line file and on a feature→feature import. Nothing
else changes.

## Phase 1 — Fix the dependency warts (1 day)

The changes that pay off regardless of everything else (fixes W1/W2 from
[16 §16.2](16-current-architecture-assessment.md)):

1. **Create `internal/kernel`** with the shared domain vocabulary:
   `ErrNotFound`, `ErrConflict`, `ErrInvalid` (validation), `newID()`,
   `ListOptions/Page`. These move *out of* `internal/repository` and
   `internal/service/errors.go`/`auth.go`.
2. **Repoint 16 service files** from `repository.ErrNotFound` to
   `kernel.ErrNotFound` (mechanical, gofmt-level diff).
3. **Break postgres→service:** the few adapter files importing `service`
   do so for sentinel errors and small helpers — they now import
   `kernel`. Store interfaces stay where they are for now (they move in
   Phase 2/3 with their features).
4. **Split nothing yet** — this phase is purely import hygiene.

Exit: `internal/service` no longer imports `internal/repository`;
`platform/postgres` no longer imports `internal/service`; all tests green.

## Phase 2 — Pilot slice: tickets (2–3 days)

Carve one feature end-to-end; it is the best pilot because it is the
most complex domain (state machine + approvals), so it proves the pattern
under stress:

```
internal/tickets/
├── tickets.go     types + service constructor + CRUD/list + visibility rules  (~350)
├── state.go       ticketActions map + Action() transition logic               (~180)
├── approvals.go   chain materialization + step decisions + inbox              (~250)
├── http.go        handlers + RegisterRoutes(r *gin.RouterGroup, ...)          (~260)
└── store.go       type Store interface (the port, consumer-side) + doc        (~80)
```

Steps, in order:

1. Define `tickets.Store` by *copying* the ticket-shaped methods out of
   `service.TicketsRepo` (consumer-defined interface — Ben Johnson's
   rule). `MemoryStore` and `platform/postgres` already implement every
   method; no adapter code changes on day 1.
2. Move `model.Ticket*` types into the `tickets` package (they are only
   used by tickets, files-attachments, and search — the latter two get a
   narrow read-only view or keep importing the exported types).
3. Move the service logic out of `service/tickets.go` (611 lines) into
   the three files above — the seams already exist as comment-blocks and
   method groups.
4. Move handlers out of `httpapp/tickets.go` into `tickets/http.go`;
   registration becomes one `tickets.RegisterRoutes(api, svc)` call in
   `router.go`.
5. Move the ticket queries from the monolithic memory god-struct into
   `repository` subfile `memory_tickets.go` (package stays; file
   ownership gets local) — later phases can adopt per-feature memory
   files wholesale.
6. Extend the contract test: the tickets section of `storeContract`
   now targets `tickets.Store` for both adapters.
7. `service.Container` keeps a `*tickets.Service` field (wiring shrinks
   by one hub-entry); frontend untouched (API contract unchanged).

Exit: `internal/service/tickets.go` deleted; every new file ≤ 400; full
suite + e2e ticket flow green. **This phase writes the playbook the
remaining phases follow.**

## Phase 3 — Roll out the remaining features (1–2 days each, any order)

Apply the pilot recipe feature-by-feature. Suggested order — least
coupled first to build confidence, hubs last:

`settings → notifications → audit → apikeys → invitations → email
tokens → search → reports/analytics → files → export → webhooks → jobs →
mfa → tokens(JWT) → users → orgs/RBAC → auth → streams`

Conventions that emerge as standard by ~the third feature (write them
down as they stabilize):

- `internal/<feature>/http.go` exposes `RegisterRoutes(rg *gin.RouterGroup, svc *Service)`
  — the Mat Ryer pattern; `router.go` becomes a short list of
  registrations instead of a 409-line table.
- `bootstrap` wiring per feature becomes `feature.New(deps)` constructors;
  `container.go` slims to composing feature constructors.
- Per-feature memory adapter files (`repository/memory_<feature>.go`)
  replace slices of the god-struct; the god-struct itself dissolves into
  small stores with a shared mutex where genuinely needed.
- Shared HTTP helpers stay in `internal/httpx` (middleware, problem+json,
  ETag, SSE, auth extraction) — cross-cutting code is *not* a feature.

Exit: no `service/ports.go`, no flat `httpapp/` feature files; hub
(`router` + `container` + `bootstrap`) files each well under 400 lines.

## Phase 4 — Selective DDD tactics (optional, 1–2 days)

Only where the domain pays (per [17 §17.4](17-architecture-options.md)):

- `tickets/state.go` becomes a true invariant owner: `func (t *Ticket)
  Apply(a Action, actor Actor, now time.Time) ([]Event, error)` — all
  transitions through one method; the action map stays as data.
- `streams` apply stays a pure function over `(user, event)` with the
  three guards explicit — already effectively done; just document it as
  the reference pattern.
- No value-object ceremony, no domain-event bus, no aggregates elsewhere.

## Effort & risk summary

| Phase | Effort | Risk | Rollback |
|---|---|---|---|
| 0 Guardrails | ½ d | ~none | drop the CI check |
| 1 Kernel/errors | 1 d | low — compiler-guided | single revert |
| 2 Tickets pilot | 2–3 d | medium — touches contract tests | revert PR; feature untouched otherwise |
| 3 Rollout | 1–2 d × 18 | low per PR (mechanical) | one PR per feature |
| 4 DDD tactics | 1–2 d | low, optional | skip |

Total: ~2–3 focused weeks spread over small PRs. Every PR keeps the API,
DB schema, frontend and demo behavior byte-identical — this is a
structure migration, not a behavior change.

## The Feature Playbook (the goal state)

After Phase 3, adding a feature — say **Newsletter subscriptions** — is:

| Step | File you create / touch |
|---|---|
| 1. Migration | `migrations/postgres/0000NN_newsletters.{up,down}.sql` |
| 2. Queries | `queries/newsletters.sql` (CRUD + 3 pagination modes) → `make generate` |
| 3. **The feature** | `internal/newsletters/newsletters.go` (types + service), `http.go` (handlers + `RegisterRoutes`), `store.go` (`Store` interface) |
| 4. Adapters | `repository/memory_newsletters.go` + `platform/postgres` queries impl (already generated) |
| 5. Register | one line in `router.go` + one constructor call in `bootstrap` |
| 6. Contract | `api/openapi/openapi.yaml` paths → `make generate` |
| 7. Frontend | `web/src/lib/api.ts` + types + one route file + nav entry |
| 8. Tests | extend `storeContract` with the newsletters section; one handler unit test; one e2e scenario; k6 only if hot |
| 9. Docs | feature doc + matrices; ADR only if you decided something |

**Shared files you touch: exactly two, one line each** (router
registration, bootstrap wiring). Everything else is *your new folder*.
That is the whole point of the exercise.

Cross-reference: the template-level (non-refactoring) version of this
checklist lives in [15 — Template Guide §15.3](15-template-guide.md).
