## Purpose

The org-invitations capability replaces out-of-band member onboarding with the standard email offer: an admin invites an address with a role, the invitee accepts after authenticating, and membership plus the tenant role grant land atomically.

## ADDED Requirements

### Requirement: Inviting An Email

An actor holding `orgs:update` shall invite an email address with an existing role key. The invitation shall carry a single-use hashed token (7-day TTL) delivered by email, and shall be refused — with uniform conflict answers — when the address is already a member of that org or already has a pending invitation to it.

#### Scenario: Happy invite

- **WHEN** an admin invites an unaffiliated address with the requester role
- **THEN** a pending invitation exists and the email carries the accept link

#### Scenario: Refusals do not leak state

- **WHEN** the address is already a member or already invited
- **THEN** the API answers 409 and creates nothing

### Requirement: Accepting With A Session

Acceptance shall require authentication (the grant lands on the caller's account), shall succeed only for pending, unexpired tokens, shall insert the org membership idempotently, shall grant the invited role in the org's Casbin domain, and shall mark the invitation accepted. Each token works exactly once.

#### Scenario: Accept then reuse

- **WHEN** the invitee accepts and the same token is presented again
- **WHEN** the second attempt arrives
- **THEN** the first acceptance granted membership and the role, and the reuse is refused

### Requirement: Administration And Discovery

Actors with `orgs:read` shall list an org's invitations with statuses; actors with `orgs:update` shall revoke pending invitations; and a signed-in user shall see the pending invitations addressed to their account email, regardless of which org sent them.

#### Scenario: Revoke stops acceptance

- **WHEN** a pending invitation is revoked and its token is then used
- **THEN** acceptance is refused and the status stays revoked
