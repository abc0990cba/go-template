# API

OpenAPI is the source of truth for HTTP behavior. `oapi-codegen` generates the Go wire types from the contract into `internal/api/api.gen.go`; request handlers bind against them, and a CI job regenerates sqlc, oapi-codegen, and the frontend types, lints the contract, and rejects any drift.

The contract is enforced on three levels:

1. **Go handler tests** — `newTestServer` wraps the router with
   request+response validation against the spec
   (`internal/httpapp/contract_test.go`), so any handler whose wire shapes
   drift from the contract fails `make test` with a schema-level message.
2. **Frontend types** — `web/src/shared/api/schema.gen.ts` is generated from
   the spec (openapi-typescript); slices call the typed client
   (`api` + `unwrap` from `web/src/shared/api/core`), so a contract change
   fails `tsc` at the call sites.
3. **MSW fixtures** — the SPA's mocks are validated against the declared
   response schemas (`web/src/test/mocks/contract.test.ts`), and
   `make lint-spec` (vacuum) lints the contract itself (operationIds,
   success + error responses).

Files:

- `api/openapi/openapi.yaml` — the contract (v1.0.0)
- `api/openapi/openapi.cfg.yaml` — Go codegen config (models only)
- `api/openapi/.vacuum.json` — contract lint ruleset
- `internal/api/api.gen.go` — generated Go types; never edit
- `web/src/shared/api/schema.gen.ts` — generated TS types; never edit
- `/openapi.yaml` and `/docs` (Scalar) served by the app

Conventions:

- Errors are RFC 9457 problem+json (`type`, `title`, `status`, `detail`); internal error details are logged, never returned.
- Authentication: `Authorization: Bearer <token>` for the API; the `app_session` cookie for the browser.
- Collections: `GET` responses wrap pages in `{data: [...], next_cursor}`; pagination uses `mode=offset|cursor|keyset` with `limit`, `offset`, `after`, `q`.
- JSON field names are snake_case and must match the `model` struct tags and the OpenAPI schemas.
- Use nouns and standard methods; route paths stay unversioned (`/api/...`) until a breaking change forces a `/api/v2` split — the contract version lives in `info.version`.

Rules:

- Change the contract and regenerate (`make generate`) in the same commit as the handler change.
- The contract documents wire shapes; semantic validation (email formats, password policy) stays in the kernel so the API keeps answering business-rule violations with problem+json.
- Document every endpoint with an `operationId`; add new schemas under `components/schemas`.
- Do not expose internal error details; map domain errors through the single HTTP mapper.
