## Why

The repository did not compile (`internal/platform/oauth` imports `golang.org/x/oauth2`, which was missing from `go.mod`), had no git history, no CI, no linting, and silently swallowed errors in several layers. A template for production use must start from a green, enforced baseline.

## What Changes

- Fix the broken build and move the module to Go 1.26.
- Add GitHub Actions CI: vet, build, race-enabled tests, golangci-lint.
- Add `.golangci.yml` and `make lint`/`make vet`/`make coverage` targets; resolve all findings, including doc comments for exported symbols.
- Make configuration fail fast: invalid or non-positive values are startup errors, log level is configurable, `.env.example` documents every variable.
- Introduce the error contract: `service.ValidationError` joins the sentinel errors; a single mapper assigns HTTP statuses (404/409/401/400), logs internal errors, and never leaks their messages to clients.
- Replace the timestamp-based request id with a crypto-random one that honors upstream `X-Request-ID`; rename the mislabeled `rateLimit` middleware to `bodyLimit`.
- Fix the bearer-token parsing bug (`Authorization: Bearer <token>` was never stripped) and add snake_case JSON tags to all models.
- Stop discarding errors in UI/job handlers; render client-safe messages in templates and log the rest.
- Add baseline test suites: memory store (table-driven pagination and conflicts), config validation, middleware, and an httptest API flow.

## Capabilities

### New Capabilities

- `engineering-experience`

## Impact

- No capability behavior changes; existing specs keep their scenarios.
- Traceability matrices are untouched: no requirement rows changed status; the new tests guard existing requirements (AUTH-001/002, USER-001, API-001).
