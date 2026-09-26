# Testing

Test layers:

- Unit tests for services and reliability helpers.
- Integration tests with Testcontainers for PostgreSQL, Redis, ClickHouse, and job processing.
- E2E tests for browser/API flows.
- Performance tests with k6.

Commands:

```bash
make test
make test-integration
make test-e2e
make test-performance
```

Feature completion requires tests listed in the matrices.
