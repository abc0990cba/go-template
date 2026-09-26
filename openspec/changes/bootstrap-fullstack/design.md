## Context

See `proposal.md`.

## Goals

- Keep the app runnable immediately.
- Preserve production boundaries.
- Make future PostgreSQL, Redis, ClickHouse, asynq, sqlc, and templ work straightforward.
- Use OpenSpec and matrices as the working contract.

## Decisions

- Use an in-memory repository for the first scaffold so the UI can be tested without infrastructure.
- Keep PostgreSQL migrations and sqlc queries in the repo from the start.
- Use server-rendered UI with HTMX available for progressive enhancement.
- Add reliability helpers before integrating the external enrichment provider deeply.
- Keep worker and API as separate commands.

## Risks / Trade-offs

- In-memory state is not production storage. Mitigation: repository boundary and migrations are present.
- OAuth is currently provider-link scaffolding, not real OAuth. Mitigation: isolate auth provider work in the `user-auth` capability.
- asynq is not wired yet. Mitigation: job service shape and worker command are present.

## Migration Plan

No production migration exists. This is the bootstrap change.
