## Why

Enterprise deployments expect 2FA. The template's password + session auth has no second factor, and the token-auth work that follows needs a challenge-aware password grant. TOTP (RFC 6238) needs no SMS vendor and works with every authenticator app.

## What Changes

- Add `mfa_factors`, `mfa_recovery_codes`, `mfa_challenges` (migration 000014, sqlc queries, memory + postgres adapters behind a `MFARepo` port).
- Add `internal/service/mfa.go`: enrollment (base32 secret + otpauth URI, SPA renders the QR), confirmation by proving a valid code, recovery codes stored as sha256 hashes and shown exactly once, and a login challenge bridge — the password step returns a short-lived challenge token instead of a session, completed by a valid TOTP or recovery code (attempt-capped, pruned by the worker).
- Login flow: `POST /api/auth/login` answers 401 `{mfa_required, challenge}` for enrolled accounts and 403 `{mfa_enrollment_required}` when the `mfa.required` policy is on and the account has no confirmed factor. `POST /api/auth/mfa/challenge` finishes the login and behaves like the old one-step endpoint.
- Self-service endpoints: status, enroll, verify, disable (password + current code both required). Admin endpoint: `DELETE /api/users/{id}/mfa` (permission `users:update`, audited, notifies the user). New `mfa.required` setting in the admin console.
- SPA: a two-step login card, a profile section (QR via `qrcode`, manual secret entry, one-time recovery codes display), i18n-safe English copy matching the existing profile cards.
- Impersonation bypasses MFA by design (the operator already holds `impersonation:execute` and both start and stop are audited) — documented.

## Capabilities

### New Capabilities

- `mfa`

## Impact

- Login handler switches from `Auth.Login` to `MFA.Authenticate`; `Auth.Login` stays for internal callers and tests.
- OpenAPI gains six paths + five schemas; `internal/api/api.gen.go` regenerated.
- Worker gains a challenge-prune loop alongside the idempotency pruner.
- web: `qrcode` dependency; login page and profile page changes.
