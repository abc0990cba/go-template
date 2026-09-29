# ADR 0003 — sqlc (Typed SQL Codegen) Instead of an ORM

**Status:** Accepted

## Context

PostgreSQL is the system of record; data access must be reviewable,
performant, and honest about SQL semantics (window-free keyset
pagination, `FOR UPDATE SKIP LOCKED`, upserts, generated tsvector
columns, partial indexes). ORMs hide exactly the parts that matter and
pull a query language that is worse than SQL.

## Decision

All data access is **hand-written SQL in `queries/*.sql`** (23 files,
190 named queries), compiled by **sqlc** into typed Go
(`internal/platform/postgres/db`, pgx/v5 driver). Migrations are plain
SQL applied by golang-migrate at boot. Configuration maps
`timestamptz → time.Time` and `jsonb → string` so JSON handling stays in
the service layer.

House rules the 190 queries follow: explicit `:one/:many/:exec/:execrows`
annotations (execrows makes "updated 0 rows" a distinguishable 404);
three pagination modes written side-by-side; annotations stay minimal —
indexes live in migrations, next to the schema they serve.

## Consequences

**Positive:** every query is reviewable SQL and benchmarkable with
`EXPLAIN`; generated Go is statically typed with zero reflection; no
lazy-loading surprises or N+1 opacity; the same SQL text documents the
access patterns.

**Negative:** schema changes require editing queries + regenerating
(`make generate`) — deliberate friction; dynamic queries (arbitrary
filter combinations) need either per-combination queries or careful
composability, which is why list endpoints expose a fixed, documented
filter set.

## Alternatives considered

- *GORM/ent* — runtime query building, weaker SQL fidelity, large
  dependency; rejected.
- *Raw pgx in code* — maximum power, zero reviewability of the query
  inventory; sqlc is raw pgx plus compile-time checking.
- *Stored procedures* — moves logic out of version-controlled Go and
  complicates testing; rejected.
