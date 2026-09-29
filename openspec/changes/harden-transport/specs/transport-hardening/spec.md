## Purpose

The transport-hardening capability defines how the HTTP surface behaves under adversarial or degraded conditions: every request is traceable, bodies are capped regardless of framing, identities are throttled individually, and the API can be deliberately paused without losing administrative control.

## ADDED Requirements

### Requirement: Inbound Server Spans

Every HTTP request shall start a server span named after the matched route pattern, carrying the request method, route, and response status; trace context shall continue inbound W3C traceparent headers. When tracing is disabled the middleware shall add no measurable cost.

#### Scenario: Span per request

- **WHEN** the OTLP endpoint is configured and a request is served
- **THEN** a server span named after the route pattern is exported with the final status code

#### Scenario: No provider, no cost

- **WHEN** `APP_OTLP_ENDPOINT` is unset
- **THEN** requests are served through the no-op tracer with no exporter traffic

### Requirement: Bounded Request Bodies

The API shall reject request bodies larger than the configured cap with 413 problem+json regardless of whether the client sent Content-Length or chunked framing.

#### Scenario: Chunked oversize body

- **WHEN** a client streams more than the cap with `Transfer-Encoding: chunked`
- **THEN** the response is 413 problem+json

#### Scenario: Pre-declared oversize body

- **WHEN** a client declares Content-Length above the cap
- **THEN** the response is 413 problem+json before the handler runs

### Requirement: HSTS On Secure Deployments

When `APP_COOKIE_SECURE` is enabled every response shall carry `Strict-Transport-Security: max-age=31536000; includeSubDomains`.

#### Scenario: Secure mode emits HSTS

- **WHEN** the app runs with `APP_COOKIE_SECURE=1`
- **THEN** responses carry the HSTS header

### Requirement: File-Based Secrets

For `APP_DATABASE_URL`, `APP_SMTP_PASSWORD`, and `APP_CLICKHOUSE_PASSWORD`, the loader shall accept a `<VAR>_FILE` environment variable pointing at a file whose contents are the secret; specifying both forms shall fail startup, as shall an unreadable file.

#### Scenario: Database URL from a mounted secret

- **WHEN** only `APP_DATABASE_URL_FILE` is set and points at a readable file
- **THEN** the app starts with the file contents as the URL

#### Scenario: Ambiguous secret

- **WHEN** both `APP_DATABASE_URL` and `APP_DATABASE_URL_FILE` are set
- **THEN** startup fails with a mutual-exclusion error

### Requirement: Per-Identity Rate Limiting

Authenticated requests shall be throttled per principal (user id, or API-key subject) with a dedicated budget in addition to the per-IP API budget; the scope shall fail open when the limiter backend is unavailable.

#### Scenario: One hot account does not exhaust the IP budget

- **WHEN** a single user exceeds the per-user budget while staying under the per-IP budget
- **THEN** only that user's requests receive 429 with `Retry-After`

### Requirement: Maintenance Mode

Setting `maintenance.enabled` shall make every API route answer 503 problem+json with `Retry-After`, except login, logout, and the settings API (which stays permission-gated so an administrator can disable the flag). Health, metrics, version, OpenAPI, and the SPA shall remain served. The flag may be memoized briefly; changes shall take effect without a restart.

#### Scenario: Maintenance blocks the API

- **WHEN** the flag is on and a client calls a tickets endpoint
- **THEN** the response is 503 problem+json with a `Retry-After` header

#### Scenario: Admin retains control

- **WHEN** the flag is on and an administrator updates `maintenance.enabled` to false
- **THEN** the update succeeds and the API resumes within the memoization window
