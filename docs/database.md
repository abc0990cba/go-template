# Database

PostgreSQL is the operational source of truth. The service layer depends on ports (`internal/service/ports.go`); the PostgreSQL adapter (`internal/platform/postgres`) implements them with pgx and sqlc-generated queries. Set `APP_DATABASE_URL` to use it — otherwise the app runs on the in-memory fake. The adapter applies pending migrations at startup; both adapters must pass the same contract suite (`test/integration/store_contract_test.go`).

Files:

- `migrations/postgres/000001_init.up.sql` and `000002_sessions.up.sql` (golang-migrate naming; every migration has a `.down.sql` pair)
- `queries/*.sql` (one file per aggregate)
- `sqlc.yaml` (schema lists the up-migrations sqlc should compile)
- `internal/platform/postgres/store.go` (adapter: transactions, sentinel-error mapping, health ping)

Rules:

- Add migrations for schema changes; name them `NNNNNN_name.up.sql` with a matching `.down.sql`, and add the new up-file to `sqlc.yaml` schema.
- Add SQL queries before repository implementation.
- Generate Go code with `make generate` (sqlc).
- Use keyset pagination for large tables.
- Avoid N+1 queries.
- Keep transactions explicit — a user with a password writes user, credential, and identity atomically.
- Map driver errors to the domain sentinels (`ErrNotFound`, `ErrConflict`) in the adapter, nowhere else.
