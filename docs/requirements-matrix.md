# Requirements Matrix

| ID | Feature | Status | Interface | Notes |
|---|---|---|---|---|
| AUTH-001 | Register with email/password | Implemented | UI + API | Uses Argon2id |
| AUTH-002 | Login/logout | Implemented | UI + API | Cookie session |
| AUTH-003 | Link Google/GitHub provider | Scaffolded | UI | Mock linking, OAuth callback pending |
| USER-001 | CRUD users | Implemented | UI + API create/list | Update/delete UI implemented |
| USER-002 | Offset/cursor/keyset pagination | Implemented | UI + API | SQL (sqlc) + in-memory, verified by store contract suite |
| JOB-001 | Enqueue enrichment job | Implemented | UI | In-memory queue |
| JOB-002 | Bounded concurrent external requests | Implemented | Worker service | randomuser.me client |
| REPORT-001 | Generate user report | Implemented | UI | Synchronous scaffold |
| ANALYTICS-001 | Analytics dashboard | Implemented | UI + API | In-memory summary, ClickHouse schema scaffolded |
| REL-001 | Graceful shutdown | Implemented | API + worker | Context-based |
| API-001 | OpenAPI contract | Implemented | `/openapi.yaml`, `/docs` | Scalar page |
