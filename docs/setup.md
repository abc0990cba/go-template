# Setup

## Requirements

- Go 1.26+
- Docker, for PostgreSQL, Redis, and ClickHouse
- Optional: golangci-lint, for `make lint`
- Optional: OpenSpec CLI
- Optional: k6 for performance tests

`.env.example` lists every supported variable with defaults; the app reads them from the environment and runs with zero configuration locally.

## Start The App

```bash
make run
```

The app runs on `http://localhost:8080`.

## Start The Stack

```bash
make docker-up
```

This starts API, worker, PostgreSQL, Redis, and ClickHouse.

## First User

1. Open `/register`.
2. Create an account with email and password.
3. Use the sidebar to test users, enrichment, jobs, reports, analytics, and API docs.
