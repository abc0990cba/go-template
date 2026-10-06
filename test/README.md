# test/ — cross-cutting test suites

Tests that span the whole binary (unit tests live next to the code under `internal/`).

## Contents

| Directory | What it is |
| --- | --- |
| `integration/` | Integration tests on real dependencies via testcontainers: Postgres, Redis, ClickHouse, Kafka, NATS, MinIO. `store_contract_test.go` pins both store adapters (Postgres and in-memory) to one contract, plus extra coverage for audit immutability, file store, multipart uploads, queues and streams. |
| `e2e/` | Full-stack black-box tests against a running server, gated behind a build tag (happy path, RBAC, smoke, streams). |
| `performance/` | k6 load scripts: `smoke.js`, `login.js`, `users-list.js`, `analytics.js`, `enrichment.js` (shared helpers in `common.js`). |

## Running

| Suite | Command | Requires |
| --- | --- | --- |
| Integration | `make test-integration` | Docker (for testcontainers) |
| E2E | `make test-e2e` | A built, running stack (see docs) |
| Performance | `make test-performance` | A seeded stack to run k6 against |

## Docs

- Testing overview & matrix guide: [../docs/testing/README.md](../docs/testing/README.md)
- Testing guide (how each suite works, when it runs in CI): [../docs/testing/testing.md](../docs/testing/testing.md)
- Coverage matrices: [../docs/testing/test-matrix.md](../docs/testing/test-matrix.md), [../docs/testing/performance.md](../docs/testing/performance.md)
