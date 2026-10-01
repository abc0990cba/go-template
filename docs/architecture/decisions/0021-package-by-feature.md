# ADR 0021 — Reorganize to Package-by-Feature (Vertical Slices)

**Status:** Accepted — Implemented 2026-10-01 (all 19 feature packages migrated in one session; the compact changelog lives in the [Migration Record](../19-migration-record.md), the plan in the [roadmap](../18-migration-roadmap.md), the layout in [04 §4.1](../04-components-and-code-map.md))

## Context

The codebase is a disciplined layered monolith ([assessment](../16-current-architecture-assessment.md)):
business logic imports no infrastructure, memory and Postgres adapters are
contract-proven equivalent, errors map centrally. But measured reality
shows three organizational costs against the project's stated goals —
*no abstraction for its own sake, files ≤ 400 lines, feature locality*:

1. Sentinel domain errors live in the in-memory adapter package, so 16 of
   24 service files import an adapter (`repository.ErrNotFound`).
2. `platform/postgres` imports `service` (~11 files) because store
   interfaces are defined in the producer package, not the consumer.
3. Every feature is scattered across five directories and plugs into
   three hub files (`ports.go` 23 interfaces, `router.go` 409 lines,
   `container.go`) — the opposite of "I know which files to take for a
   new feature". Several files already exceed a 400-line budget
   (`rbac.go` 687, `tickets.go` 611, `streams.go` 420, `router.go` 409).

The Go community's canonical guidance — Ben Johnson ([Standard Package
Layout](https://medium.com/@benbjohnson/standard-package-layout-7a7bb7d155f6)),
Mat Ryer ([How I write HTTP services in Go after 13
years](https://matryer.com/posts/how-i-write-http-services-in-go-after-13-years/)),
Simon Brown ([modular monoliths / package by
feature](https://simonbrown.je/modular-monolith/)), Three Dots Labs
([Go With The Domain](https://threedots.tech/go-with-the-domain/)) —
converges on feature-first packages with consumer-defined interfaces and
minimal ceremony.

## Decision

Adopt **package-by-feature vertical slices** as the target structure and
migrate incrementally per the [roadmap](../18-migration-roadmap.md):

- Each feature owns `internal/<feature>/` containing its domain types,
  service logic (split along natural seams to respect the ≤ 400-line
  budget), HTTP handlers with a `RegisterRoutes` constructor, and its
  `Store` interface **defined next to its consumer**.
- A minimal `internal/kernel` carries shared domain vocabulary
  (`ErrNotFound`, `ErrConflict`, `newID`, paging types) and imports
  nothing internal.
- `internal/platform/` adapters remain as-is (sqlc/Postgres, queue,
  cache, mailer, brokers, filestore) and implement per-feature `Store`
  interfaces; the memory adapter stays, per-feature files.
- Interfaces exist only where a second implementation is real (the
  store duality) or a test seam demands it; the 23-interface `ports.go`
  is dissolved. No command bus, no use-case layer, no wholesale DDD —
  tactical patterns (aggregate-style state machine, pure apply function)
  confined to tickets and streams.

Supersedes the *layout* aspect of ADR 0001 (hexagonal ports & adapters
remains the discipline at each feature's edges; what changes is the
cohesion axis: feature first, layer second).

## Consequences

**Positive:** feature locality — new features mean one new package plus
two one-line hub registrations; the 400-line budget becomes natural
rather than enforced; hub files stop growing; merge conflicts concentrate
in feature folders; each slice is a clean extraction seam if a service
split ever happens (without paying for one now).

**Negative:** a multi-week mechanical migration (18 feature carve-outs)
with contract-test churn per step; import-cycle discipline needed when
features reference each other (mitigated by `kernel` and narrow
consumer-side interfaces); during the transition, the tree is
intentionally inconsistent — the roadmap's phase order and CI guardrails
exist to bound that window.

**Neutral:** public behavior — API, schema, frontend, demo data — is
unchanged by construction; every phase is behavior-neutral and verified
by the existing suites.

## Alternatives considered

- *Keep layering, fix warts only* — does not deliver feature locality;
  hubs regrow.
- *Strict Clean Architecture* — rejected: DTO/mapper ceremony per feature
  buys swaps this product will never make.
- *Wholesale DDD tactics* — rejected: the domain's complexity is
  concentrated in two seams; tactics are applied only there.
- *Command-bus vertical slices* — rejected: Mediatr-style dispatch is
  abstraction-for-abstraction's-sake in Go.
- *Microservices now* — rejected: monolith-first stands (ADR 0002); the
  slice seams keep the option cheap.
