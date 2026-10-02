# Test Matrix

| Requirement | Unit | Integration | E2E | Notes |
|---|---|---|---|---|
| AUTH-001 | `internal/service/auth_test.go` | `internal/httpapp/router_test.go` | `test/e2e/happy_path_test.go` | Register covered |
| AUTH-002 | `internal/service/auth_test.go` | `internal/httpapp/router_test.go` | `test/e2e/happy_path_test.go` | Login covered |
| AUTH-003 | `internal/service/auth_test.go` | `internal/httpapp/security_test.go` (mock provider) | Planned | Real OAuth flow covered |
| USER-001 | `internal/repository/memory_test.go` | `test/integration/store_contract_test.go` | `test/e2e/happy_path_test.go` | Store contract + e2e |
| USER-002 | `internal/repository/memory_test.go` | `test/integration/store_contract_test.go` | Planned | Offset/cursor/keyset on both adapters |
| JOB-001 | Planned | `test/integration/queue_test.go` | `test/e2e/happy_path_test.go` | Redis e2e + queue dedup |
| JOB-002 | `internal/reliability/retry_test.go` | Planned | Planned | Bounded runner covered |
| REL-001 | `internal/reliability/retry_test.go` | Planned | Planned | Retry behavior covered |
| REL-002 | `internal/reliability/bulkhead_test.go` | Planned | Planned | Limit, reject, ctx-cancel, observers covered |
| OBS-001 | `internal/observ/slogtrace_test.go` | Planned | Planned | Handler stamps IDs; sampler mapping |
| REL-003 | `internal/repository/memory_test.go` | `test/integration/streams_contract_test.go` (prune subtest) | Planned | Dedupe-vs-sequence-guard after sweep, both twins |
| WEB-AUTH | `web/src/lib/auth.test.ts`, `web/src/lib/api.test.ts` | `web/src/routes/login.test.tsx` | `web/e2e/auth.spec.ts` | Guards, login protocol, MFA step, redirect-back |
| WEB-TICKETS | `web/src/hooks/use-paged-list.test.tsx` | `web/src/routes/_client.tickets.test.tsx` | `web/e2e/tickets.spec.ts` | Paging, error toasts, SSE push, create flow |
| WEB-GUARDS | `web/src/lib/auth.test.ts` | `web/src/routes/admin.test.tsx` | Planned | admin_console:access gate |
| WEB-SHELL | — | `web/src/components/data-table/pagination-bar.test.tsx` | `web/e2e/landing.spec.ts` | Table controls + landing/SPA serving split |
| WEB-APPROVALS | — | `web/src/routes/_client.approvals.test.tsx` | `web/e2e/approvals.spec.ts` | Decision dialog, reject-requires-comment, decided tab, two-role chain |
| WEB-PROFILE | — | `web/src/routes/_client.profile.test.tsx` | Planned | OAuth identities, TOTP lifecycle, API keys, oauth_error |
| WEB-AUTH-FLOWS | — | `web/src/routes/auth-flows.test.tsx` | `web/e2e/auth.spec.ts` (forgot/reset) | Register, reset, verify, invitation |
| WEB-ADMIN-USERS | — | `web/src/routes/admin.users.test.tsx` | `web/e2e/admin-users.spec.ts` | CRUD dialogs, roles dialog, server-side search |
| WEB-CONTRACT | `web/src/test/mocks/contract.test.ts` | — | — | MSW handlers ⊆ OpenAPI spec |
| WEB-A11Y | — | — | `web/e2e/a11y.spec.ts` | axe serious/critical gate; contrast debt allowlisted |
| WEB-VISUAL | — | — | `web/e2e/visual.spec.ts` | Landing + login snapshots, Linux baselines |
| WEB-I18N | `web/src/lib/i18n.test.tsx` | — | — | en/ru parity, en fallback, locale switch |
