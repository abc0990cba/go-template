# ADR 0007 — Casbin RBAC with Domains as Tenants

**Status:** Accepted

## Context

The product needs multi-tenancy (organizations), platform-level roles,
per-tenant role grants, wildcard permissions (`tickets:*`), and an
admin-editable policy — before any business feature. Hand-rolling this is
a classic source of subtle authorization bugs.

## Decision

Use **Casbin v2** with the `(sub, dom, obj, act)` model:
`g` lines assign roles per domain, `p` lines grant
`resource:action` per domain; the matcher allows roles/policies from the
platform domain `*` to apply everywhere, and `keyMatch2` provides
wildcards. Domains are organization ids; `*` is the platform. Policies
persist in `casbin_rule` through an autosaving adapter over the
`RuleStore` port (so memory and Postgres both work). A
`SyncedEnforcer` is shared; **a nil enforcer denies everything**.

On top of the enforcer, the service layer exposes `EffectivePermissions`
(wildcards expanded against a closed permission catalog, cached 30 s,
invalidated on policy mutation) — this expanded list drives both API
guard middleware and the SPA's `useCan`.

Built-in system roles (`superadmin/admin/manager/requester/approver/viewer`)
seed idempotently; first registration becomes `superadmin`, later ones
`viewer`.

## Consequences

**Positive:** multi-tenancy is a model feature, not a query convention;
policy edits are data changes, safe at runtime, fully audited; the raw
policy editor (`/api/policies`) makes the engine transparent instead of
magic.

**Negative:** Casbin's model semantics are a learning curve (documented
in [08](../08-flows-authorization-and-tenancy.md)); expanded-permission
caching introduces a ≤ 30 s propagation window after policy changes
(accepted; explicit invalidation covers the common admin flows).

## Alternatives considered

- *Hand-rolled permission tables* — every team underestimates wildcard +
  inheritance + domain interactions; this is exactly the code you don't
  want to own.
- *OPA / external policy engine* — a separate service before the first
  microservice; wrong size for this template.
- *SpiceDB/Zanzibar-style* — for planet-scale relationship models; not
  here.
