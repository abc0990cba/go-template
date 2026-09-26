# Database

PostgreSQL is the operational source of truth.

Files:

- `migrations/postgres/001_init.sql`
- `queries/users.sql`
- `sqlc.yaml`

Rules:

- Add migrations for schema changes.
- Add SQL queries before repository implementation.
- Generate Go code with sqlc.
- Use keyset pagination for large tables.
- Avoid N+1 queries.
- Keep transactions explicit.
