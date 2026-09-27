## 1. Data Layer

- [x] 1.1 Migration 000012: `ticket_types`, `tickets`, `ticket_approvals`, `ticket_comments`, `ticket_events` (+ down); append to `sqlc.yaml`
- [x] 1.2 `queries/tickets.sql`: CRUD, scoped listings (mine/all/approver inbox/decisions), comments, events, demo reset
- [x] 1.3 Model structs in `internal/model/model.go`
- [x] 1.4 `TicketsRepo` port in `internal/service/ports.go` + compile-time assertions
- [x] 1.5 Memory store implementation
- [x] 1.6 Postgres store implementation; `make generate`

## 2. Domain Logic

- [x] 2.1 RBAC: `tickets`/`ticket_types` resources, `requester`/`approver` system roles, policies
- [x] 2.2 `internal/service/tickets.go`: transition table, role guards, decision comments, events, notifier, RoleChecker
- [x] 2.3 Container/Deps wiring; bootstrap Repos bundle; cmd/api, cmd/worker, cmd/seed-demo constructors

## 3. HTTP Surface

- [x] 3.1 OpenAPI paths/schemas; regenerate `api.gen.go`
- [x] 3.2 `internal/httpapp/tickets.go` handlers + routes with permission guards and audit

## 4. Demo Data

- [x] 4.1 Ticket types, role grants (requester/approver/manager) in demo orgs
- [x] 4.2 ~120 tickets across statuses with consistent approvals, comments, events; reset support

## 5. Frontend

- [x] 5.1 Types, api client methods, usePagedList resource, permissions
- [x] 5.2 "My tickets" page (scope tabs, create dialog, status badges)
- [x] 5.3 "Approvals" page (pending inbox, decision dialogs with required reject comment, my decisions)
- [x] 5.4 Ticket detail page (role-aware actions, terminal banner withdrawn vs rejected, approval checklist, timeline, comments)
- [x] 5.5 Nav entries with permission gating; RU/EN i18n

## 6. Verification And Docs

- [x] 6.1 Router tests: requester and approver lifecycles, permission separation, invalid transitions
- [x] 6.2 Store contract section for TicketsRepo (memory + postgres)
- [x] 6.3 E2E happy-path extension; docs/tickets.md, README, traceability; openspec validate

## 7. Client App / Admin Console Split

- [x] 7.1 `admin_console:access` catalog resource + role grants; `demo_mode` public flag
- [x] 7.2 Route restructure: `/admin` layout (guarded, sidebar shell) and `_client` layout (top navbar shell); client home
- [x] 7.3 IdentityChip, demo account picker on login and switch dialogs; ticket-types admin page
- [x] 7.4 Withdraw allowed from approved; tests + spec updated
