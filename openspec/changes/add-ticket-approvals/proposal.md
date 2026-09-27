## Why

The template covers CRUD, jobs, RBAC, and analytics, but has no multi-entity business workflow. A ticket domain with approvals exercises a guarded state machine, role-scoped interfaces, and 1:N related entities — the patterns most real products need next — and gives the demo dataset a live human process to show.

## What Changes

- Add `tickets`, `ticket_types` (with jsonb approval steps), `ticket_approvals`, `ticket_comments`, `ticket_events` (migration 000012, sqlc queries, memory + postgres adapters behind one `TicketsRepo` port).
- Add a coded state machine in `internal/service/tickets.go`: draft → in_review → approved → completed, with distinct terminal states `rejected` (by an approver, with a mandatory comment) and `withdrawn` (by the requester). Approve/reject guard on the current step's Casbin role in the org domain plus the `tickets:approve` permission.
- Add system roles `requester` and `approver` to the RBAC catalog so the role matrix explicitly models the two sides.
- Separate HTTP surfaces: `GET /api/tickets?scope=my|all` (requester's own tickets; org-wide needs `tickets:read_all`), `GET /api/approvals?state=pending|decided` (approver inbox and decision history), one transition endpoint `POST /api/tickets/{id}/actions`, plus comments and ticket-type CRUD.
- New SPA surfaces: "My tickets" page for requesters, a separate "Approvals" inbox page for approvers (nav entry gated by `tickets:approve`), and a role-aware detail page whose action bar, terminal banner, timeline, and status badges distinguish "withdrawn by requester" from "rejected by approver" in RU and EN.
- Demo data: three ticket types, requester/approver/manager role assignments in demo orgs, ~120 tickets across all statuses with consistent approval steps, comments, and event history.
- Notifications: submit notifies the current step's approvers, reject/withdraw notify the requester; every transition is audited, emitted to the analytics trail, and published over SSE.

## Capabilities

### New Capabilities

- `ticket-approvals`

## Impact

- New permission catalog entries (`tickets`, `ticket_types`) and two system roles; the roles page matrix and policy editor show them automatically.
- Registrations land as `viewer` as before; org members get `requester` semantics through the role grants seeded for demo orgs (or manual assignment).
- OpenAPI regenerated (`internal/api/api.gen.go` is CI-enforced); `web/src` gains types, client methods, and two new routes.
- Demo seed grows by ticket entities; `resetDemoData` clears them via the `demo-` prefix.
