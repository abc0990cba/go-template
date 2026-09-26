## Why

The repository boundary existed, but only an in-memory fake implemented it, and API and worker processes never shared state. Production shape requires a real database path without coupling services to driver details.

## What Changes

- Define store ports (AuthRepo, UsersRepo, JobsRepo, ReportsRepo, AnalyticsRepo, Pinger) in the service layer; both adapters satisfy them at compile time.
- Add a PostgreSQL adapter: pgx pool, sqlc-generated queries for every aggregate, transactions for user+password+identity, sentinel-error mapping (404/409), and a readiness ping.
- Run schema migrations at startup with golang-migrate (iofs over `migrations/postgres`); rename migrations to the `NNNNNN_name.up/down.sql` convention with real down scripts; align the schema with the domain model (identity name/avatar/last_login_at, sessions, OAuth state, case-insensitive unique email, keyset-ordered index).
- Select the adapter in one place (`internal/bootstrap`): PostgreSQL when `APP_DATABASE_URL` is set, the memory fake otherwise.
- `/health/ready` now pings the database and returns 503 on failure.
- Contract test suite (testcontainers, Postgres) runs identical scenarios against both adapters; wired into CI as a separate job.
- docker-compose: postgres healthcheck, named volume, `DATABASE_URL` for api and worker, depends_on conditions.

## Capabilities

### New Capabilities

- `persistence`

## Impact

- USER-002 pagination is no longer in-memory-only: offset/cursor/keyset have SQL implementations verified by the contract suite.
- Matrices: integration tests now exist for the store; performance scripts for pagination remain planned (performance OpenSpec change).
