# Two-Factor Authentication (TOTP)

The template ships RFC 6238 time-based one-time passwords as a second sign-in
factor: enroll from the profile page, confirm with a code from your
authenticator app, and every later login asks for that code (or a recovery
code).

## Flows

**Enroll** (profile → Two-factor authentication):

1. `POST /api/auth/mfa/enroll` stores an unconfirmed factor and returns the
   base32 secret + `otpauth://` URI (the SPA renders the QR with `qrcode`;
   the secret doubles as the manual-entry path).
2. `POST /api/auth/mfa/verify` with a current 6-digit code confirms the
   factor and returns **10 recovery codes — displayed exactly once**, stored
   only as sha256 hashes.

**Login with MFA**: `POST /api/auth/login` answers
`401 {"mfa_required": true, "challenge": "…"}` instead of a session.
`POST /api/auth/mfa/challenge` with the challenge plus a TOTP **or** one
recovery code completes the login (session cookie + bearer token, exactly
like the one-step response). Challenges live 5 minutes, allow at most 5
wrong codes, and are swept by the worker every 10 minutes.

**Disable**: `POST /api/auth/mfa/disable` requires the password *and* a
current code, so a stolen browser session cannot weaken the account alone.

**Admin reset**: `DELETE /api/users/{id}/mfa` (`users:update` permission)
idempotently wipes factor + codes, writes an audit entry, and notifies the
user — the "lost my phone" path.

## Policy

The admin console's `mfa.required` setting blocks login for accounts without
a confirmed factor: the password step answers
`403 {"mfa_enrollment_required": true}`. Enroll before the policy flips; the
wrong-password answer stays a uniform 401 so the flag cannot be used to
enumerate accounts.

## Design notes

- Secrets, challenge tokens, and recovery codes are never stored in clear
  text (challenges/codes as sha256, same scheme as email tokens); the TOTP
  secret itself is required server-side to validate codes and is excluded
  from every API response (`json:"-"`).
- Recovery codes are lowercase base32 (no ambiguous characters), accepted
  with dashes/spaces pasted in.
- Impersonation bypasses the challenge by design: the operator already holds
  `impersonation:execute`, and both start and stop are audited.
- The API-key and session resolution paths are unchanged — MFA guards the
  *login* event, not the session.

## Try it locally

```bash
make docker-up        # or the hybrid dev mode from docs/quickstart.md
```

1. Sign in as any demo account → Profile → "Set up two-factor".
2. Scan the QR with Google Authenticator / Aegis / 1Password (or paste the
   secret into https://totp.app for a quick test).
3. Confirm with the 6-digit code, **save the recovery codes**.
4. Sign out and sign in again — the login card asks for the code.
5. Toggle `mfa.required` in Admin → Settings, sign in with a different demo
   account, and observe the enrollment-required answer.
