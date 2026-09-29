## 1. Data Layer

- [x] 1.1 Migration 000014 `mfa_factors` / `mfa_recovery_codes` / `mfa_challenges` (+ down); sqlc schema entry
- [x] 1.2 `queries/mfa.sql` (factor upsert/confirm/get/delete, codes, challenges, prune)
- [x] 1.3 Model structs; `MFARepo` port + compile-time assertions
- [x] 1.4 Memory + postgres adapters; `make generate`

## 2. Domain Logic

- [x] 2.1 `internal/service/mfa.go`: Authenticate (password step → session | challenge | enrollment gate), CompleteChallenge (TOTP or single-use recovery code, attempt cap), Enroll/VerifyEnrollment/Disable/Reset/Status, challenge pruning
- [x] 2.2 Container + bootstrap + binaries wiring; `mfa.required` setting definition; worker prune loop

## 3. HTTP Surface

- [x] 3.1 OpenAPI paths/schemas; regenerate
- [x] 3.2 Handlers + routes (challenge under the auth rate budget; admin reset under `users:update`); login handler rewired; `user_logged_in` analytics preserved on both paths

## 4. Frontend

- [x] 4.1 api.ts: loginStep outcome protocol (ok | mfa_required | enrollment_required), MFA methods
- [x] 4.2 Two-step login card; profile MFA card (QR, manual secret, one-time recovery codes, disable)

## 5. Verification And Docs

- [x] 5.1 Service tests (enroll/verify/disable, challenge flow, attempts cap, recovery single-use, required-policy gate) + router tests (full API flow, admin reset permission)
- [x] 5.2 docs/mfa.md, traceability matrix
