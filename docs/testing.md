# Testing

Test layers, all wired into CI:

- Unit tests for services, stores, middleware, and reliability helpers (`make test`, race-enabled).
- Store contract tests: one scenario suite runs against both the in-memory fake and a real PostgreSQL container via testcontainers (`make test-integration`).
- Queue end-to-end test: enqueue through Redis and process with the real asynq mux (`make test-integration`).
- API happy-path e2e against the live compose stack; skips itself when the stack is down (`make test-e2e` after `make docker-up`).
- Performance tests with k6 (`make test-performance`).

Coverage:

- `make coverage` measures `./internal/...` with the race detector.
- `make coverage-check` enforces a floor on the whole hand-written tree:
  `-coverpkg=./internal/...` counts never-tested packages as zero, and
  generated files (OpenAPI types, sqlc queries — guarded by the codegen job
  instead) are filtered from the profile. CI runs it in the build-test job.
  The floor (`COVERAGE_FLOOR`, default 46) tracks the measured baseline —
  currently ~50%: raise it as coverage grows (target 70%). The httpapp
  handlers and the Postgres adapters are the largest uncovered areas.

Rules:

- Feature completion requires tests listed in `docs/test-matrix.md`.
- Integration tests must not require a manually started stack; e2e tests must skip, not fail, when the stack is absent.
- New load-bearing code paths arrive with at least one table-driven test.
