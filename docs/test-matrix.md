# Test Matrix

| Requirement | Unit | Integration | E2E | Notes |
|---|---|---|---|---|
| AUTH-001 | `internal/service/auth_test.go` | Planned | Planned | Register path covered |
| AUTH-002 | `internal/service/auth_test.go` | Planned | Planned | Login path covered |
| AUTH-003 | `internal/service/auth_test.go` | Planned | Planned | Mock provider link covered |
| USER-001 | `internal/repository/memory_test.go` | `test/integration/store_contract_test.go` | Planned | Store contract covered on both adapters |
| USER-002 | `internal/repository/memory_test.go` | `test/integration/store_contract_test.go` | Planned | Offset/cursor/keyset covered on both adapters |
| JOB-001 | Planned | Planned | Planned | Add asynq adapter tests |
| JOB-002 | `internal/reliability/retry_test.go` | Planned | Planned | Bounded runner covered |
| REL-001 | `internal/reliability/retry_test.go` | Planned | Planned | Retry behavior covered |
