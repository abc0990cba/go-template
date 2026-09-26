# Go Template Playground

Production-shaped Go application template for learning and reuse.

The project is intentionally small enough to understand, but it includes the patterns expected from a real service:

- Gin HTTP API
- Full-stack UI for manual testing
- OpenAPI contract
- OpenSpec workflow
- CI (GitHub Actions) with vet, golangci-lint, and race-enabled tests
- PostgreSQL migrations and sqlc query layout
- Redis/asynq-ready background job design
- ClickHouse analytics schema
- Unit, integration, e2e, and performance test structure
- Reliability rules: graceful shutdown, context propagation, bounded concurrency, retry limits, and health checks

## Run Locally

```bash
make run
```

Open:

- App UI: http://localhost:8080
- API docs: http://localhost:8080/docs
- Health: http://localhost:8080/health/live

Register a user, then use the dashboard, profile, users, enrichment, jobs, reports, and analytics pages.

## Development Commands

```bash
make vet
make lint
make test
make coverage
make test-performance
make openspec-validate
make docker-up
```

The first implementation uses an in-memory repository so the app can be explored immediately. PostgreSQL, Redis, ClickHouse, sqlc, and asynq integration points are scaffolded for the next OpenSpec changes.

## Project Rules

- Start each feature with an OpenSpec change.
- Update OpenAPI for HTTP behavior.
- Update SQL migrations and sqlc queries for database behavior.
- Add tests and performance coverage for each user-facing or load-bearing feature.
- Update traceability matrices before a feature is considered complete.
