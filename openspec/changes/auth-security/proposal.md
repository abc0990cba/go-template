## Why

Provider linking was a mock that fabricated identities, sessions never expired, HTML forms had no CSRF protection, cookies lacked Secure/SameSite, and auth endpoints had no rate limiting — all documented as required by docs/auth.md.

## What Changes

- Real Google/GitHub OAuth2 authorization-code linking: `/auth/{provider}/login` (requires an active session) and `/callback` exchange and link the verified `(provider, subject)`; state is single-use with TTL; provider endpoint overrides support mock-provider tests.
- Sessions gain a server-side TTL with sliding renewal and logout-all; mock `LinkProvider` is removed in favor of `LinkIdentity` with a provider profile.
- CSRF double-submit protection on every HTML form; the JSON API stays exempt.
- Security headers middleware (CSP, nosniff, frame denial, referrer policy).
- Per-IP token-bucket rate limiting on login/register for UI and API (429 on overflow), with shared state across all throttled routes.
- Cookies: HttpOnly, SameSite=Lax, configurable Secure flag.

## Capabilities

### New Capabilities

- `oauth-linking`
- `web-security`

## Impact

- The profile page's mock link buttons become real OAuth redirects; the login page explains that linking requires a session.
- Store contract suite extends to session TTL/sliding/logout-all.
