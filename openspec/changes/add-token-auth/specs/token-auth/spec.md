## Purpose

The token-auth capability gives non-browser clients an OAuth-style grant: short-lived identity-only access JWTs verified statelessly per request, backed by rotating refresh tokens whose reuse is treated as a breach signal. The feature is opt-in per deployment and MFA-aware.

## ADDED Requirements

### Requirement: Token Grant Endpoints

When `APP_JWT_SECRET` is configured, `POST /api/auth/token` shall accept `grant_type=password` (credentials → tokens), `grant_type=mfa_challenge` (challenge + code → tokens), and `grant_type=refresh_token` (rotation); `POST /api/auth/token/revoke` shall revoke a refresh token idempotently and silently for unknown tokens. Without a secret both endpoints shall answer 501.

#### Scenario: Password grant issues a working access token

- **WHEN** valid credentials are exchanged
- **THEN** the response carries an access token that authenticates authorized API routes and an unused refresh token

#### Scenario: Opt-in contract

- **WHEN** no secret is configured and a client calls the token endpoint
- **THEN** the answer is 501 and nothing is issued

### Requirement: Rotation With Reuse Detection

Every successful refresh shall retire the presented token and mint a new one in the same family. Presenting a retired token shall revoke every live token in its family and answer 401.

#### Scenario: Rotate once

- **WHEN** a live refresh token is presented
- **THEN** the response carries a different refresh token and the old one stops working

#### Scenario: Reuse kills the family

- **WHEN** a retired refresh token is presented
- **THEN** the answer is 401 with a reuse title and the successor token from the same family is also dead

### Requirement: Identity-Only Access Tokens

Access tokens shall be HS256 JWTs carrying issuer, subject, token type, and time claims — never roles, permissions, or organization scope; authorization shall be resolved server-side per request. Verification shall reject foreign secrets, tampered payloads, wrong issuers, and expired tokens.

#### Scenario: No privilege data in the token

- **WHEN** an access token is decoded
- **THEN** it contains no role, permission, or organization claims, and a permission change applies to the next request without re-issuing

### Requirement: MFA-Aware Grants

The password grant shall behave like the browser login: enrolled accounts receive a challenge instead of tokens (completed via `grant_type=mfa_challenge`), and the `mfa.required` policy blocks accounts without a confirmed factor.

#### Scenario: Enrolled account gets a challenge

- **WHEN** an MFA-enrolled account runs the password grant
- **THEN** the response is 401 with `mfa_required` and a challenge, and no tokens are issued
