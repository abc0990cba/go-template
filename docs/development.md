# Development

## Workflow

1. Create or update an OpenSpec change.
2. Update API/spec/docs first.
3. Implement the smallest vertical slice.
4. Add tests.
5. Add or update performance coverage when the feature affects latency, throughput, or concurrency.
6. Update the traceability matrix.

## Quality Gates

Every change must pass before merge; CI runs the same set:

```bash
make vet    # go vet
make lint   # golangci-lint, see .golangci.yml
make test   # go test -race ./...
```

House rules enforced by the linter and review:

- Configuration errors fail startup instead of silently falling back.
- Domain errors (`repository.ErrNotFound`, `repository.ErrConflict`, `service.ValidationError`, `service.ErrInvalidCredentials`) map to HTTP status codes in one place; internal error details are logged, never sent to clients.
- UI handlers never discard errors: client-safe messages render in the page, unexpected ones are logged.
- JSON responses are snake_case via `model` struct tags.

## Local Mode

The current runtime uses an in-memory repository. This keeps the template easy to run while preserving the production boundaries:

- controllers
- services
- repositories
- jobs
- analytics
- reliability helpers

Replace the in-memory repository with PostgreSQL/sqlc in a dedicated OpenSpec change.
