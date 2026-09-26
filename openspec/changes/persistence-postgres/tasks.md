## 1. Ports And Adapters

- [x] 1.1 Define service ports (AuthRepo, UsersRepo, JobsRepo, ReportsRepo, AnalyticsRepo, Pinger) with compile-time assertions for the memory fake
- [x] 1.2 Rename migrations to golang-migrate convention with down scripts; align schema (identity columns, sessions, OAuth state, case-insensitive email, keyset index)
- [x] 1.3 Write sqlc queries for every aggregate; `make generate` runs sqlc
- [x] 1.4 Implement the PostgreSQL adapter: tx for user+password+identity, sentinel mapping, ping, close
- [x] 1.5 Select the adapter in `internal/bootstrap`; wire api and worker

## 2. Verification

- [x] 2.1 Contract suite (testcontainers) against memory and PostgreSQL
- [x] 2.2 CI job for the contract suite
- [x] 2.3 docker-compose: healthcheck, volume, DATABASE_URL, depends_on conditions

## 3. Docs

- [x] 3.1 Update docs/database.md, README status, .env.example
