# Go Template Playground

Production-shaped Go application template for learning and reuse.

The project is intentionally small enough to understand, but it includes the patterns expected from a real service:

- Gin HTTP API
- Full-stack UI for manual testing
- OpenAPI contract
- OpenSpec workflow
- CI (GitHub Actions) with vet, golangci-lint, and race-enabled tests
- PostgreSQL persistence (pgx + sqlc + migrations) behind service ports
- Redis/asynq job queue with a real worker process and idempotent enqueueing
- Observability: structured logs, Prometheus metrics, OpenTelemetry traces
- PostgreSQL migrations and sqlc query layout
- Redis/asynq-ready background job design
- ClickHouse analytics schema
- Unit, integration, e2e, and performance test structure
- Reliability rules: graceful shutdown, context propagation, bounded concurrency, retry limits, and health checks

## Run Locally

```bash
make docker-up          # full stack
# or the hybrid dev mode - see docs/quickstart.md
docker compose up -d postgres redis
APP_DATABASE_URL="postgres://app:app@127.0.0.1:55432/app?sslmode=disable" \
APP_REDIS_ADDR="127.0.0.1:6379" \
APP_DEMO_EMAIL="demo@go-template.local" \
APP_DEMO_PASSWORD="demo-password-123" \
APP_COOKIE_SECURE=0 \
go run ./cmd/api
```

Demo account (seeded at startup when `APP_DEMO_EMAIL`/`APP_DEMO_PASSWORD` are set): `demo@go-template.local` / `demo-password-123`. Full runbook: [docs/quickstart.md](docs/quickstart.md).

Open:

- App UI: http://localhost:8080
- API docs: http://localhost:8080/docs
- Metrics: http://localhost:8080/metrics
- Version: http://localhost:8080/version
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

The default run uses an in-memory repository so the app can be explored immediately. Set `APP_DATABASE_URL` and `APP_REDIS_ADDR` (see `.env.example`) — or run `make docker-up` — for PostgreSQL (pgx + sqlc + startup migrations) and the Redis/asynq queue with a real worker process. Add `--profile observability` for Jaeger, Prometheus, and Grafana; see `docs/observability.md`. ClickHouse analytics and templ UI land in upcoming OpenSpec changes.

## Project Rules

- Start each feature with an OpenSpec change.
- Update OpenAPI for HTTP behavior.
- Update SQL migrations and sqlc queries for database behavior.
- Add tests and performance coverage for each user-facing or load-bearing feature.
- Update traceability matrices before a feature is considered complete.
