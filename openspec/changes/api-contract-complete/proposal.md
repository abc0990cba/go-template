## Why

The contract described 7 operations with no response schemas and no error bodies, while the app served far more endpoints; nothing verified contract and code agreed.

## What Changes

- Rewrite `api/openapi/openapi.yaml` (v1.0.0): every served operation documented, response schemas for all endpoints, RFC 9457 problem schema, bearer security on protected operations, pagination parameters.
- Add the missing API surface: `GET/PUT/DELETE /api/users/{id}`, `GET /api/jobs`, `POST /api/jobs/enrichment`, `GET/POST /api/reports`.
- Generate Go wire types with oapi-codegen (models only, `internal/api/api.gen.go`); request handlers bind against the generated structs.
- `make generate` runs sqlc and oapi-codegen; CI job regenerates and rejects drift.

## Capabilities

### New Capabilities

- `api-contract-sync`

## Impact

- USER-001 update/delete now exists over the API, not only the UI; JOB/REPORT enqueue/list are API-reachable.
