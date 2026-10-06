# RBAC: Casbin with domains (organizations)

## Model

Policies live in Casbin with the RBAC + domains model:

```
p = sub, dom, obj, act      # permission: subject, domain, object, action
g = _, _, _                 # assignment: user, role, domain
```

- The domain `*` is the whole platform; every organization is its own domain
  (its id).
- A rule applies when the subject holds the rule's role in the request's
  domain **or** in `*`, the domains match (`*` is the wildcard), and the
  object/action match under Casbin's `keyMatch2` wildcards (`users:*`, `*`).
- `p, superadmin, *, *, *` — full access for the superadmin role.

The model text and the adapter live in `internal/platform/rbac`
(`ModelText`, `Adapter`). The adapter persists rules through the
`rbac.RuleStore` port (table `casbin_rule`), so the in-memory mode and
PostgreSQL are interchangeable.

## Permission catalog

Resources and actions are declared in `internal/rbac/rbac.go`
(`Resources`). New protected routes:

1. add the resource/action to the catalog;
2. guard the route with `RequirePermission("resource", "action")` in your
   feature's `RegisterHTTP` — the guard closure is defined in
   `internal/httpapp/rbac.go` and wired through `internal/httpapp/router.go`;
3. hide UI elements with `useCan()(resource, action)`.

## Request scope

The `orgDomain` middleware picks each request's domain: the `X-Org-Id`
header (requires membership in that organization or full access — unknown
ids answer 400, non-members 403), otherwise the user's single organization,
otherwise `*`. The UI keeps the selected scope in localStorage and sends the
header with every request.

## System roles

| Role | Permissions (domain `*`) |
|------|--------------------------|
| superadmin | everything, including the raw policy editor |
| admin | users/roles/orgs/settings/webhooks management, jobs and reports, full tickets and streams, analytics, audit |
| manager | read/update users, `jobs:execute`, reports, full tickets (including `read_all` and `approve`), streams replay, analytics |
| requester | files, tracks, and completes own tickets |
| approver | reads tickets and decides their approval steps |
| viewer | read-only jobs/reports/enrichments/analytics/tickets |

Role metadata lives in the `roles` table (`is_system` protects system roles
from deletion). Defaults are seeded at startup (`EnsureSystemRoles`).

## Bootstrap permissions

The first registered account gets `superadmin` and every later
self-registration lands as `viewer` — change that in `grantRegistrationRole`
(`internal/auth/http.go`). The demo dataset then layers platform roles on
top: `demo-u-0001@demo.example` is `admin`, `demo-u-0002` `manager`,
`demo-u-0003` `viewer`, `demo-u-0004` `requester`, `demo-u-0005` `approver`,
and `demo-u-0006` holds both `requester` and `approver`
(`internal/bootstrap/demodata_orgs.go`).

## Key files

| What | Where |
|-----|-----|
| Model + adapter + enforcer | `internal/platform/rbac/` |
| Service (roles, permissions, orgs) | `internal/rbac/` |
| Domain middleware + permission guard | `internal/httpapp/rbac.go` |
| HTTP handlers (roles, orgs, policies) | `internal/rbac/http.go`, `internal/rbac/http_orgs.go` |
| UI pages | `web/src/routes/admin.roles.tsx`, `admin.policies.tsx`, `admin.orgs.tsx` |
| Permission hook | `web/src/shared/hooks/use-permissions.ts` |
| Contract | `api/openapi/openapi.yaml` → tag `rbac` |

## See also

- [ADR 0007: Casbin RBAC with domains](../architecture/decisions/0007-casbin-rbac-with-domains.md)
- [Architecture chapter 08: Authorization & Tenancy](../architecture/08-flows-authorization-and-tenancy.md)
