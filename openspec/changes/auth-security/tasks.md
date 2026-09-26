## 1. Sessions

- [x] 1.1 TTL + sliding renewal in both adapters; contract suite extended
- [x] 1.2 DeleteSessionsForUser + LogoutAll service + profile action

## 2. OAuth Linking

- [x] 2.1 Manager endpoint overrides for mock providers
- [x] 2.2 /auth/{provider}/login + /callback handlers; single-use state; session-bound
- [x] 2.3 LinkIdentity(provider profile) replaces mock LinkProvider; real buttons in UI

## 3. Web Security

- [x] 3.1 CSRF double-submit on HTML forms (API exempt); token rendered by every page
- [x] 3.2 Security headers middleware; NoRoute 404 with nosniff
- [x] 3.3 Shared per-IP token-bucket limiter on login/register (UI + API)
- [x] 3.4 Cookie hardening: HttpOnly, SameSite=Lax, APP_COOKIE_SECURE

## 4. Verification And Docs

- [x] 4.1 Mock-provider OAuth flow test (redirect → exchange → link → replay rejected)
- [x] 4.2 CSRF, headers, rate-limit tests
- [x] 4.3 docs/auth.md rewritten; compose/env updated
