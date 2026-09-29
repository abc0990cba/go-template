# Token Auth — JWT Access + Refresh Rotation

For mobile apps and third-party integrations the template exposes an
OAuth-style token grant **alongside** the browser cookie sessions and
`gtpl_` API keys. It is opt-in per deployment: set `APP_JWT_SECRET`
(≥32 bytes; `_FILE` variant supported) — without it the endpoints answer
501 and the rest of the app is unaffected.

```
POST /api/auth/token
{"grant_type":"password","email":"…","password":"…"}
→ 200 {"access_token":"…","token_type":"Bearer","expires_in":900,"refresh_token":"…"}
```

- **Access tokens** are HS256 JWTs, TTL `APP_JWT_ACCESS_TTL` (default 15m).
  Claims are identity-only (`iss`, `sub`, `typ`, `iat`, `exp`, `jti`) — no
  roles, no org. Authorization is still resolved server-side (Casbin) on
  every request, so permission changes apply immediately and a leaked token
  grants nothing beyond the holder's current permissions.
- **Refresh tokens** rotate on every use (`grant_type=refresh_token`), TTL
  `APP_JWT_REFRESH_TTL` (default 30d). They live server-side as sha256
  hashes grouped into **families**: presenting a *rotated* token is treated
  as a breach — the whole family is revoked instantly and the answer is 401
  `refresh token reuse detected`.
- **MFA-aware**: enrolled accounts get
  `401 {"mfa_required":true,"challenge":"…"}` from the password grant and
  finish with `{"grant_type":"mfa_challenge","challenge":"…","code":"…"}`;
  the `mfa.required` policy blocks unenrolled accounts exactly like the
  browser login.
- **Revocation**: `POST /api/auth/token/revoke {"refresh_token":…}` kills
  one token (204, also for unknown tokens — no oracle). Log out everywhere
  for token clients: `RevokeAllForUser` (used by the profile flow's
  password-change path).

Send the access token as `Authorization: Bearer <jwt>`; the router accepts
sessions, `gtpl_` keys, and JWTs in that header on the same routes.

## Why no role claims?

Embedding roles would freeze authorization at grant time and require token
revocation lists to take effect — the opposite of the stateless property
that makes short access tokens cheap. Identity-only claims + server-side
authZ is the pattern that survives audits.

## Try it locally

```bash
APP_JWT_SECRET="local-dev-secret-0123456789abcdef" go run ./cmd/api
```

```bash
curl -s localhost:8080/api/auth/token -d '{"grant_type":"password","email":"demo@go-template.local","password":"demo-password-123"}' | jq
curl -s localhost:8080/api/users?limit=1 -H "Authorization: Bearer <access_token>" | jq
curl -s localhost:8080/api/auth/token -d '{"grant_type":"refresh_token","refresh_token":"<refresh>"}' | jq
# reuse the first refresh token now → 401 reuse, and the successor dies too
```
