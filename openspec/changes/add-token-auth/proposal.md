## Why

Cookie sessions fit the SPA, and `gtpl_` API keys fit personal scripting, but mobile apps and third-party integrations expect an OAuth-style token grant with short-lived access tokens. The template needs the standard rotation + reuse-detection machinery, MFA-aware from day one.

## What Changes

- Add `refresh_tokens` (migration 000015, sqlc queries, memory + postgres adapters behind `TokenRepo`): rotation chains grouped into families, sha256-hashed, device metadata recorded.
- Add `internal/service/tokens.go`: `PasswordGrant` (credentials → tokens; MFA-enrolled accounts get the challenge bridge, `mfa.required` blocks unenrolled), `CompleteChallengeGrant`, `RefreshGrant` (rotate on every use; presenting a rotated token revokes the whole family), `Revoke`, `RevokeAllForUser`, `Prune`.
- HS256 access JWTs (identity claims only: iss/sub/typ/iat/exp/jti — no roles, no org), TTLs via `APP_JWT_ACCESS_TTL` (15m) / `APP_JWT_REFRESH_TTL` (30d); the feature is opt-in via `APP_JWT_SECRET` (`_FILE` supported, ≥32 bytes) and the endpoints answer 501 without it.
- HTTP: `POST /api/auth/token` (grant_type password | mfa_challenge | refresh_token), `POST /api/auth/token/revoke`; the router accepts JWT-shaped bearers on the authorized group next to sessions and API keys. `bearerJWT` security scheme in OpenAPI.
- Worker sweeps long-expired refresh tokens hourly.

## Capabilities

### New Capabilities

- `token-auth`

## Impact

- `apiAuthRequired` grows a JWT branch (two-dot heuristic → VerifyAccess → user load); sessions and API keys keep priority semantics unchanged.
- Config grows three validated knobs; `.env.example` documents them.
