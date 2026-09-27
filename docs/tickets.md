# Tickets & Approvals

The tickets domain adds a human approval workflow to the template: requesters
file tickets, approvers decide on them step by step, and the two sides work
in deliberately separated interfaces.

## The state machine

```
                 submit                (all steps approve)      complete
  draft ────────────────▶ in_review ────────────────────▶ approved ────────▶ completed
    │                       │  ▲        │
    │                       │  │        │ reject (comment required)
    │                       │  └────┐   ▼
    │ withdraw              │       rejected ◀─┐
    ▼                       │            │     │ reopen (author)
  withdrawn (terminal)      └── approve ─┘─────┘   (next step / final approval)
```

| Status      | Meaning |
|-------------|---------|
| `draft`     | Created by the requester, editable and deletable by them |
| `in_review` | Submitted; the approval chain decides step by step |
| `approved`  | Every step approved; the requester can deliver the result |
| `completed` | The requester closed the loop ("сдал" the result) |
| `rejected`  | **An approver refused it** — with a mandatory comment; the requester can rework and resubmit |
| `withdrawn` | **The requester closed it themselves** — terminal, no approver declined anything |

`rejected` and `withdrawn` are deliberately different statuses: one means
"an approver said no", the other means "the author changed their mind". The
API, the UI badges, the terminal banners, and both UI languages keep the two
apart.

Transitions are declared in `internal/service/tickets.go` (`ticketActions`).
Illegal transitions answer HTTP 409.

## Roles and interfaces

| Role | Tickets permissions | Interface |
|------|--------------------|-----------|
| `requester` | read, create, update, delete — enforced to **own** tickets server-side | **My Tickets** (`/tickets`) |
| `approver`  | read, approve (+ service access to tickets waiting on their step) | **Approvals** (`/approvals`) |
| `manager`   | requester + approver + `read_all`; reads ticket types | Both pages + the "All (org)" tab |
| `admin`     | `tickets:*`, `ticket_types:*` | Everything |
| `viewer`    | read (own) | My Tickets (read-only) |

- **My Tickets** shows only the caller's tickets (`GET /api/tickets`,
  `scope=my` by default). Org-wide listing (`scope=all`) requires the
  `tickets:read_all` permission.
- **Approvals** (`GET /api/approvals?state=pending`) lists only tickets whose
  *current* step names one of the caller's Casbin roles — decided server-side.
  `state=decided` returns the caller's decision history.
- The detail page shows action buttons based on the caller's side (author vs
  current-step approver) and re-checks nothing: every action re-runs the
  guards server-side.

## Approval chains

Ticket types (`ticket_types`) carry an ordered `steps` JSON array:

```json
[
  { "name": "Manager review", "approver_role": "manager" },
  { "name": "Finance approval", "approver_role": "approver" }
]
```

Submitting a ticket materializes one pending `ticket_approvals` row per step.
`approve` requires the caller to hold the step's Casbin role in the active
domain (admins bypass via full access) plus the `tickets:approve` catalog
permission; approving the last step moves the ticket to `approved`. `reject`
requires a comment, marks later steps `skipped`, and records `decided_by` /
`decided_at` on the ticket.

## Data layout

Migration `000012_tickets`: `ticket_types` (steps as jsonb), `tickets`
(per-org sequence `number`, displayed as `TCK-0001`), `ticket_approvals`
(unique per ticket+step), `ticket_comments`, `ticket_events` (timeline).
All tables cascade on ticket delete.

Every transition appends a `ticket_events` row, an audit entry
(`ticket.submit`, `ticket.reject`, …), an analytics event
(`ticket_approved`, …), and publishes `ticket.updated` over SSE.

## Permissions wiring

New resources in the RBAC catalog (`internal/service/rbac.go`):

- `tickets`: read, read_all, create, update, delete, approve
- `ticket_types`: read, create, update, delete

New system roles `requester` and `approver` seed with default platform
policies; custom roles can be granted any of the catalog actions from the
Roles page.

## Demo data

`APP_DEMO_SEED=1` (or `make seed-demo`) seeds three ticket types (expense,
access request, vacation), grants `requester` to demo org members and
`approver` to every second member, and writes 120 tickets spread over all
statuses — rejected ones carry the rejecting step's comment, withdrawn ones
have no approver decisions. Reset with `make seed-demo-reset`.

## Tests

- `internal/httpapp/tickets_test.go` — requester and approver lifecycles,
  the withdrawn/rejected distinction, scope separation, invalid transitions
- `test/integration/store_contract_test.go` — the TicketsRepo contract
  against both the memory store and PostgreSQL
- `test/e2e/happy_path_test.go` — `TestTicketApprovalFlow` against the live
  compose stack (self-skipping)
