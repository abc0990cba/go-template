# Testing

Test layers, all wired into CI:

- Unit tests for services, stores, middleware, and reliability helpers (`make test`, race-enabled).
- Store contract tests: one scenario suite runs against both the in-memory fake and a real PostgreSQL container via testcontainers (`make test-integration`).
- Queue end-to-end test: enqueue through Redis and process with the real asynq mux (`make test-integration`).
- API happy-path e2e against the live compose stack; skips itself when the stack is down (`make test-e2e` after `make docker-up`).
- Performance tests with k6 (`make test-performance`).

Coverage:

- `make coverage` measures `./internal/...` with the race detector.
- CI enforces a floor of 55% on internal packages (target 70%); current baseline is ~60% — the httpapp handlers and the OAuth manager are the largest uncovered areas.

Rules:

- Feature completion requires tests listed in `docs/test-matrix.md`.
- Integration tests must not require a manually started stack; e2e tests must skip, not fail, when the stack is absent.
- New load-bearing code paths arrive with at least one table-driven test.
