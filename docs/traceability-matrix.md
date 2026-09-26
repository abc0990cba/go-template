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
