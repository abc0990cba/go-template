# ADR 0005 — Opaque Server-Side Sessions for Browsers; JWT Only for API/Mobile

**Status:** Accepted

## Context

Browsers need logout-now, "log out everywhere", impersonation, and MFA
challenges — all forms of *server-controlled* session state. Stateless
JWTs in cookies fight every one of those (revocation lists re-introduce
state anyway) and push you toward cookie flags that are a standing XSS/
CSRF liability. Meanwhile mobile and third-party API clients genuinely
benefit from short-lived stateless credentials.

## Decision

Two credentials for two audiences:

- **Browsers:** opaque 16-byte random token in an `HttpOnly`,
  `SameSite=Lax` cookie (`app_session`); the session row lives in
  Postgres with **sliding expiry**; revocation is a DELETE. The same
  token is accepted as `Authorization: Bearer` for scripting convenience.
- **API/mobile:** OAuth-style grants (`password`, `mfa_challenge`,
  `refresh_token`) issuing HS256 access tokens (15 min, identity claims
  only — authorization stays server-side) plus rotating refresh tokens
  with **family-based reuse detection**: presenting a rotated token kills
  the whole family.
- **Machine integrations:** `gtpl_` API keys — hashed, prefix-lookup,
  show-once, revocable.

The auth middleware dispatches on token shape (prefix `gtpl_` → API key;
exactly two dots → JWT; else session), so all three coexist on every
route.

## Consequences

**Positive:** instant revocation and impersonation semantics for the
surface where they matter; JWT endpoints answer 501 unless a real secret
is configured (no default-secret footgun); the reuse-detection family is
a genuinely production-grade pattern in 100 lines.

**Negative:** every API request hits the session store (one indexed
lookup — mitigated by the 30 s `user:` cache for follow-on lookups);
two token systems to explain (the docs pair [07](../07-flows-authentication-and-identity.md)
with the demo script exists for exactly this).

## Alternatives considered

- *JWT-in-cookie everywhere* — rejected: revocation, impersonation and
  sliding expiry all become hacks.
- *Sessions only* — mobile/API clients pollute the session table and lose
  offline token refresh semantics.
- *PASETO / opaque-PASETO hybrids* — no benefit over opaque tokens for a
  server-side-session design.
