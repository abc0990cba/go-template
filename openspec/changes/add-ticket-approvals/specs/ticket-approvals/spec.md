## Purpose

The ticket-approvals capability defines a human approval workflow: requesters file and track tickets, approvers decide on them step by step, and the two sides work in clearly separated interfaces.

## ADDED Requirements

### Requirement: Ticket Lifecycle State Machine

A ticket shall move only along the declared transitions: submit (draft → in_review), approve (in_review → next step or approved), reject (in_review → rejected), withdraw (draft|in_review|approved → withdrawn), complete (approved → completed), reopen (rejected → draft). Any other transition is rejected with a conflict error. The requester may withdraw in every active state — including approved — but not once a terminal status (rejected, withdrawn, completed) was recorded.

#### Scenario: Requester submits and completes

- **WHEN** the requester submits a draft and every approval step later approves it
- **THEN** the ticket ends `approved` and the requester can still complete it

#### Scenario: Invalid transition

- **WHEN** an actor tries to approve a draft or reopen a withdrawn ticket
- **THEN** the API answers 409 and the ticket keeps its status

### Requirement: Distinct Terminal Outcomes

A ticket closed by its requester shall be `withdrawn` and a ticket refused by an approver shall be `rejected`, with the deciding actor, timestamp, and (for rejections) mandatory comment stored and surfaced in the UI in both UI languages.

#### Scenario: Withdrawal is not a rejection

- **WHEN** the requester withdraws an in-review ticket
- **THEN** the status is `withdrawn`, no approval step records a decision, and the UI shows "Withdrawn by requester"

#### Scenario: Rejection records the decision

- **WHEN** an approver rejects without a comment
- **THEN** the API refuses the action; with a comment the ticket becomes `rejected` and the UI shows the approver, step, and comment

### Requirement: Step-Based Approval Guards

Approve and reject shall require the caller to hold the current step's approver role in the organization domain and the `tickets:approve` permission; steps decide in order and later steps stay pending until their turn.

#### Scenario: Wrong role cannot decide

- **WHEN** a user without the current step's approver role tries to approve
- **THEN** the API answers 403 and the pending step remains pending

### Requirement: Separated Requester And Approver Interfaces

The API shall scope listings by side: `scope=my` returns only the caller's tickets, org-wide listings require `tickets:read_all`, and the approver inbox returns only tickets waiting on one of the caller's roles at their current step, with a decided-history view.

#### Scenario: Requester sees only own tickets

- **WHEN** a requester lists tickets without `read_all`
- **THEN** the response contains only tickets they created, and `scope=all` answers 403

#### Scenario: Approver inbox is role-scoped

- **WHEN** an approver opens the approvals inbox
- **THEN** only tickets whose current step matches one of their org roles appear

### Requirement: Configurable Approval Chains

Ticket types shall carry an ordered list of approval steps, each naming a Casbin role; submitting a ticket materializes one pending approval row per step.

#### Scenario: Two-step chain decides in order

- **WHEN** a type declares steps manager → approver and the ticket is submitted
- **THEN** the manager step must approve before the approver step becomes decidable


### Requirement: Separated Admin Console And Client Workspace

The SPA shall expose two explicit areas: the user-facing client workspace at the root paths (`/`, `/tickets`, `/approvals`, …) and the admin console under the `/admin` prefix, guarded by the `admin_console:access` permission at the layout level and re-checked server-side by every admin API. The client area renders its own shell (top navbar, personal home); the admin area renders the console shell (sidebar, system pages).

#### Scenario: Requester cannot enter the admin console

- **WHEN** an account without `admin_console:access` opens `/admin/users`
- **THEN** the layout redirects to the client home, and the admin APIs behind it answer 403 regardless of the UI

#### Scenario: Requester withdraws an approved ticket

- **WHEN** the requester withdraws a ticket in the `approved` state
- **THEN** the ticket becomes `withdrawn`; withdrawing a `completed`, `rejected`, or already `withdrawn` ticket answers 409
