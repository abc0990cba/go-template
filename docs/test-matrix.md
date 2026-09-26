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
