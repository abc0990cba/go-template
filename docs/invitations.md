# Organization Invitations

Members join a tenant by accepting an emailed offer instead of an admin
pasting their user id.

## Flow

1. **Admin** (orgs:update): Orgs → Seats → *Invite by email* — or
   `POST /api/orgs/{id}/invitations {"email":"…","role":"requester"}`.
   The role must exist (validated against the roles catalog); the email must
   not already be a member and must not have a pending invite (uniform 409s
   so the endpoint cannot probe state).
2. **Email** goes out through the shared mailer (Mailpit at
   http://localhost:8025 in compose; log fallback without SMTP) with a
   7-day, single-use accept link: `/accept-invitation?token=…`.
3. **Invitee** opens the link, signs in (or registers) first if needed, and
   accepts: the org membership is inserted idempotently and the invited role
   is granted in the org's Casbin domain — the same primitives as manual
   member management, so removal/role flows behave identically afterwards.
4. **Discoverability**: `GET /api/invitations` lists pending invitations
   addressed to the signed-in account's email, for the "I ignored that
   email" path.

## Admin surface

- `GET /api/orgs/{id}/invitations` — newest first, with status
  (pending/accepted/revoked/expired).
- `DELETE /api/orgs/{id}/invitations/{invId}` — revoke a pending invite.
- Every action is audited (`org.invited`, `org.invitation_accepted`,
  `org.invitation_revoked`).

The invite response echoes the raw token — a dev convenience so the flow can
be completed without reading the mail log; production clients use the email.

## Try it locally

```bash
make docker-up --profile mail   # Mailpit captures the invitation email
# Admin → Organizations → Seats → invite someone@example.com as requester
# Open http://localhost:8025 → click the accept link → sign in → accept
```
