# API

OpenAPI is the source of truth for HTTP behavior. `oapi-codegen` generates the Go wire types from the contract into `internal/api/api.gen.go`; request handlers bind against them, and a CI job regenerates both sqlc and oapi-codegen output and rejects any drift.

Files:

- `api/openapi/openapi.yaml` — the contract (v1.0.0)
- `api/openapi/openapi.cfg.yaml` — codegen config (models only)
- `internal/api/api.gen.go` — generated types; never edit
- `/openapi.yaml` and `/docs` (Scalar) served by the app

Conventions:

- Errors are RFC 9457 problem+json (`type`, `title`, `status`, `detail`); internal error details are logged, never returned.
- Authentication: `Authorization: Bearer <token>` for the API; the `app_session` cookie for the browser.
- Collections: `GET` responses wrap pages in `{data: [...], next_cursor}`; pagination uses `mode=offset|cursor|keyset` with `limit`, `offset`, `after`, `q`.
- JSON field names are snake_case and must match the `model` struct tags and the OpenAPI schemas.
- Use nouns and standard methods; route paths stay unversioned (`/api/...`) until a breaking change forces a `/api/v2` split — the contract version lives in `info.version`.

Rules:

- Change the contract and regenerate (`make generate`) in the same commit as the handler change.
- Document every endpoint with an `operationId`; add new schemas under `components/schemas`.
- Do not expose internal error details; map domain errors through the single HTTP mapper.
