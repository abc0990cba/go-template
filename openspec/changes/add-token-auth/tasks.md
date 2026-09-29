## 1. Data Layer
- [x] 1.1 Migration 000015 `refresh_tokens` (+ down); sqlc schema entry; `make generate`
- [x] 1.2 `queries/tokens.sql` (create/get/revoke/revoke-family/revoke-for-user/prune)
- [x] 1.3 Model + `TokenRepo` port + memory/postgres adapters

## 2. Domain Logic
- [x] 2.1 `internal/service/tokens.go`: grants, rotation, reuse detection, revocation, pruning, HS256 sign/verify
- [x] 2.2 MFA bridge (`BeginChallenge`, `RequiredAndMissing`); config knobs (`APP_JWT_SECRET[_FILE]`, TTLs) with validation

## 3. HTTP Surface
- [x] 3.1 OpenAPI paths/schemas/`bearerJWT` scheme; regenerate
- [x] 3.2 Handlers (`/api/auth/token`, `/api/auth/token/revoke`), JWT branch in `apiAuthRequired`, routes under the auth rate budget

## 4. Operations
- [x] 4.1 Worker prune loop; `.env.example` docs

## 5. Verification And Docs
- [x] 5.1 Service tests (grant, rotation, reuse family revocation, revoke, disabled mode, tampering, MFA bridge) + router tests (lifecycle, 501 without secret, bearer resolution)
- [x] 5.2 docs/token-auth.md, traceability matrix
