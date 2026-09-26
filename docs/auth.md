# Auth

The target auth model is hybrid:

- Cookie session for browser UI.
- Bearer tokens for API clients.

Implemented in the first scaffold:

- email/password register
- email/password login
- logout
- session cookie
- provider linking placeholders for Google and GitHub

Production rules:

- Passwords use Argon2id.
- Cookies are HTTP-only.
- OAuth linking requires an active session.
- Do not auto-link providers by email alone.
- Rate limit auth endpoints.
- Write audit events for security-sensitive actions.
