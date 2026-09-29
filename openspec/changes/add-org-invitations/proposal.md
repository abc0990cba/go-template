## Why

Adding an org member today means pasting a raw user id into the seats dialog — an admin-only, out-of-band ritual. Every SaaS product invites by email instead: the invitee signs up on their own, and membership + role land when they accept.

## What Changes

- Add `org_invitations` (migration 000017, sqlc queries, memory + postgres adapters behind `InvitationsRepo`): one row per offer with a sha256-hashed single-use token, status machine (pending → accepted | revoked | expired), 7-day TTL.
- Add `internal/service/invitations.go`: `Invite` (validates org + role key, rejects already-members and duplicate pending invites with uniform 409s, emails the accept link through the shared mailer), `Accept` (idempotent membership insert + Casbin domain role grant + status flip; expired/revoked/unknown tokens are uniform errors), `Revoke`, `ForOrg`, `Mine` (pending-by-email discoverability).
- HTTP: `POST/GET /api/orgs/{id}/invitations` (orgs:update / orgs:read), `DELETE /api/orgs/{id}/invitations/{invId}`, `GET /api/invitations`, `POST /api/invitations/accept` (self-service, authenticated).
- SPA: the org seats dialog grows an invite-by-email section (email + role + pending list with revoke); a `/accept-invitation` page lands the email link, offering sign-in-first when the visitor has no session.
- The token is echoed in the invite response for dev tooling (single-use, 7-day TTL).

## Capabilities

### New Capabilities

- `org-invitations`

## Impact

- Acceptance reuses `AddOrgMember` + `RBAC.AssignRole`, so removal flows (which revoke domain roles) keep working unchanged.
- Bug fixed en route: `forbidden()` never aborted the middleware chain — permission-denied requests answered 403 but their handlers still executed (side effects ran). Found by the invitation permission test; the abort now covers every guard using `a.forbidden`.
