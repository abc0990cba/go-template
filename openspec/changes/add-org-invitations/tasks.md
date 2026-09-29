## 1. Data Layer
- [x] 1.1 Migration 000017 `org_invitations` (+ down); sqlc; model + `InvitationsRepo` port
- [x] 1.2 Memory + postgres adapters

## 2. Domain Logic
- [x] 2.1 `internal/service/invitations.go`: Invite/Accept/Revoke/ForOrg/Mine with uniform conflict semantics and mailer integration
- [x] 2.2 Container/bootstrap wiring; `RoleByKey` validation helper on RBACService

## 3. HTTP Surface
- [x] 3.1 OpenAPI paths + schema; handlers + permission-guarded routes

## 4. Frontend
- [x] 4.1 Seats dialog: invite-by-email + role select + pending list with revoke
- [x] 4.2 `/accept-invitation` landing page with sign-in-first path

## 5. Verification And Docs
- [x] 5.1 Service tests (lifecycle, duplicates, single-use, cross-org revoke, mine-by-email) + router test (full API flow, permission guard, reuse 409)
- [x] 5.2 docs/invitations.md, traceability
