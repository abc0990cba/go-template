# Auth

The auth model is hybrid:

- Cookie session for the browser UI.
- Bearer tokens for API clients (`Authorization: Bearer <token>`).

Implemented:

- email/password register, login, logout, logout-all
- sessions with a server-side TTL (`APP_SESSION_TTL`) and sliding renewal on use
- real Google/GitHub linking via OAuth2 authorization-code flow (state stored single-use with `OAUTH_STATE_TTL`; linking requires an active session)
- CSRF protection on every HTML form (double-submit cookie, `csrf_token` field) — the JSON API is exempt
- security headers on every response (CSP, nosniff, frame denial, referrer policy)
- per-IP token-bucket rate limiting on login and register, UI and API alike (`APP_AUTH_RATE_RPS`, `APP_AUTH_RATE_BURST`)
- hard cookies: HttpOnly, `SameSite=Lax`, `Secure` per `APP_COOKIE_SECURE`

Code map: flows in `internal/platform/oauth`, handlers in `internal/httpapp/oauth.go`, form/security middleware in `internal/httpapp/security.go`, session storage in the store adapters.

Production rules:

- Passwords use Argon2id.
- Cookies are HTTP-only and SameSite=Lax; `APP_COOKIE_SECURE=1` outside local http.
- OAuth linking requires an active session; the provider's (provider, subject) pair — never user input — identifies the identity.
- Do not auto-link providers by email alone.
- Rate limit auth endpoints; exceeding the bucket returns 429.
- Audit events (`user_registered`, `user_logged_in`, `oauth_linked`) are recorded through the analytics event trail.

Testing: the OAuth flow is covered end to end against a mock provider (`internal/httpapp/security_test.go`) using the `GOOGLE_AUTH_URL`/`GOOGLE_TOKEN_URL`/`GOOGLE_USERINFO_URL` endpoint overrides.
