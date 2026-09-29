# Traceability Matrix

| Requirement | OpenSpec Capability | Code Area | API/UI | Tests | Performance |
|---|---|---|---|---|---|
| AUTH-001 | `user-auth` | `internal/service/auth.go` | `/register`, `/api/auth/register` | `auth_test.go` | Planned |
| AUTH-002 | `user-auth` | `internal/service/auth.go` | `/login`, `/api/auth/login` | `auth_test.go` | Planned |
| AUTH-003 | `user-auth` | `internal/service/auth.go` | `/profile` | `auth_test.go` | Planned |
| USER-001 | `user-management` | `internal/service/users.go` | `/users`, `/api/users` | Planned | Planned |
| JOB-001 | `background-jobs` | `internal/service/jobs.go` | `/enrichment`, `/jobs` | Planned | Planned |
| ANALYTICS-001 | `analytics-events` | `internal/service/analytics.go` | `/analytics` | Planned | Planned |
| REL-001 | `reliability` | `internal/reliability` | health endpoints | `retry_test.go` | smoke.js |
| TICKET-001 | `ticket-approvals` | `internal/service/tickets.go` | `/tickets`, `/api/tickets` | `tickets_test.go`, store contract | Planned |
| TICKET-002 | `ticket-approvals` | `internal/service/tickets.go` | `/api/tickets/{id}/actions` | `tickets_test.go` (409 on invalid transition) | Planned |
| TICKET-003 | `ticket-approvals` | `internal/service/tickets.go` | terminal banners, `ticket.status.*` i18n | `tickets_test.go` (withdrawn vs rejected) | Planned |
| TICKET-004 | `ticket-approvals` | `internal/service/tickets.go` (step guards) | `/api/tickets/{id}/actions` | `tickets_test.go` (role guard) | Planned |
| TICKET-005 | `ticket-approvals` | `internal/service/tickets.go` (scope) | `/api/tickets?scope=`, `/api/approvals` | `tickets_test.go` (scope separation) | Planned |
| TICKET-006 | `ticket-approvals` | `internal/bootstrap/demodata.go` | seeded types, roles, 120 tickets | demo seed smoke | Planned |
| TRANS-001 | `transport-hardening` | `internal/httpapp/observability.go` (tracing) | every route (server spans) | `hardening_test.go` (span recorder) | Planned |
| TRANS-002 | `transport-hardening` | `internal/httpapp/middleware.go` (bodyLimit + bind) | all JSON routes (413) | `hardening_test.go` (chunked + pre-declared) | Planned |
| TRANS-003 | `transport-hardening` | `internal/httpapp/security.go` (HSTS) | every response | `hardening_test.go` (toggle test) | Planned |
| TRANS-004 | `transport-hardening` | `internal/config/config.go` (`_FILE`) | startup validation | `hardening_test.go`, `config` tests | Planned |
| TRANS-005 | `transport-hardening` | `internal/httpapp/security.go` (identity scope) | `/api/*` per-principal 429 | `hardening_test.go` (per-user budget) | Planned |
| TRANS-006 | `transport-hardening` | `internal/httpapp/maintenance.go` | 503 + Retry-After gate | `hardening_test.go` (gate + exemptions) | Planned |
| TRANS-007 | `transport-hardening` | `internal/platform/postgres/store.go` (tuning) | pool knobs + statement timeouts | `.env.example`, integration suite | Planned |
| MFA-001 | `mfa` | `internal/service/mfa.go` (enroll/verify) | profile card, `/api/auth/mfa/enroll|verify` | `mfa_test.go` (enroll/verify/disable) | Planned |
| MFA-002 | `mfa` | `internal/service/mfa.go` (challenge) | `/api/auth/login` + `/api/auth/mfa/challenge` | `mfa_test.go` (challenge flow, attempts cap, recovery single-use) | Planned |
| MFA-003 | `mfa` | `internal/service/mfa.go` (policy) | `mfa.required` setting, 403 marker | `mfa_test.go` (required gate) | Planned |
| MFA-004 | `mfa` | `internal/httpapp/mfa.go` (admin reset) | `DELETE /api/users/{id}/mfa` | `mfa_test.go` (permission guard) | Planned |
| TOK-001 | `token-auth` | `internal/service/tokens.go` (grants) | `/api/auth/token` | `tokens_test.go` (grant/lifecycle) | Planned |
| TOK-002 | `token-auth` | `internal/service/tokens.go` (rotation) | refresh_token grant | `tokens_test.go` (rotation + reuse family) | Planned |
| TOK-003 | `token-auth` | `internal/service/tokens.go` (JWT) | bearerJWT on all routes | `tokens_test.go` (verify/tamper), `httpapp/tokens_test.go` (bearer) | Planned |
| TOK-004 | `token-auth` | `internal/service/tokens.go` (MFA bridge) | mfa_challenge grant | `tokens_test.go` (challenge bridge) | Planned |
| TOK-005 | `token-auth` | `internal/httpapp/tokens.go` (opt-in) | 501 without secret | `httpapp/tokens_test.go` (disabled) | Planned |
| CACHE-001 | `caching-search` | `internal/service/cache.go` + platform/cache | user/perms hot reads | `search_cache_test.go` (hit/singleflight), `caching_search_test.go` (invalidation) | Planned |
| CACHE-002 | `caching-search` | `internal/httpapp/etag.go` | users/jobs/tickets/analytics GETs | `caching_search_test.go` (304 + fresh tag) | Planned |
| SRCH-001 | `caching-search` | `internal/service/search.go` | `/api/search` | `search_cache_test.go`, `caching_search_test.go` (scoping) | Planned |
| SRCH-002 | `caching-search` | `internal/platform/postgres/search.go` | tsvector + GIN (migration 000016) | store contract (regression phase) | Planned |
| INV-001 | `org-invitations` | `internal/service/invitations.go` (invite) | seats dialog, `POST /api/orgs/{id}/invitations` | `invitations_test.go` (lifecycle), `httpapp/invitations_test.go` | Planned |
| INV-002 | `org-invitations` | `internal/service/invitations.go` (accept) | `/accept-invitation`, `/api/invitations/accept` | `invitations_test.go` (single-use, roles) | Planned |
| INV-003 | `org-invitations` | `internal/service/invitations.go` (revoke/list/mine) | invitations list + revoke | `invitations_test.go` (cross-org, statuses) | Planned |
| FILE-001 | `files` | `internal/platform/filestore/` | S3/memory adapters, presign | `filestore_test.go` (MinIO contract, integration tag) | Planned |
| FILE-002 | `files` | `internal/service/files.go` | `/api/files` roundtrip + caps | `httpapp/files_test.go` (roundtrip, delete) | Planned |
| FILE-003 | `files` | `internal/service/files.go` (visibility) | ticket attachments | `httpapp/files_test.go` (hidden ticket, unknown ref) | Planned |
| EXP-001 | `data-export` | `internal/service/export.go` | `POST/GET /api/users/me/export` | `export_test.go` (roundtrip), `httpapp/export_test.go` (degraded mode) | Planned |
| EXP-002 | `data-export` | `internal/service/export.go` (download) | `GET .../download` | `export_test.go` (ownership, pruning) | Planned |
