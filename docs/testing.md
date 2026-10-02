# Testing

Test layers, all wired into CI:

- Unit tests for services, stores, middleware, and reliability helpers (`make test`, race-enabled).
- Store contract tests: one scenario suite runs against both the in-memory fake and a real PostgreSQL container via testcontainers (`make test-integration`).
- Queue end-to-end test: enqueue through Redis and process with the real asynq mux (`make test-integration`).
- API happy-path e2e against the live compose stack; skips itself when the stack is down (`make test-e2e` after `make docker-up`).
- Performance tests with k6 (`make test-performance`).

Coverage:

- `make coverage` measures `./internal/...` with the race detector.
- `make coverage-check` enforces a floor on the whole hand-written tree:
  `-coverpkg=./internal/...` counts never-tested packages as zero, and
  generated files (OpenAPI types, sqlc queries — guarded by the codegen job
  instead) are filtered from the profile. CI runs it in the build-test job.
  The floor (`COVERAGE_FLOOR`, default 46) tracks the measured baseline —
  currently ~50%: raise it as coverage grows (target 70%). The httpapp
  handlers and the Postgres adapters are the largest uncovered areas.

Frontend pyramid (`web/`, mirrors the backend's shape):

- Static base: `tsc --noEmit`, oxlint, oxfmt gate the `web` CI job before any test runs.
- Unit tests for the request pipeline and guards: problem+json parsing, the
  X-Org-Id header, the login outcome protocol, route-guard redirects
  (`make web-test`, colocated `*.test.ts(x)`).
- Component tests for forms and table controls through Testing Library
  (react-hook-form + zod messages, pagination bar switching).
- Integration tests render full route files through the real TanStack route
  tree with the API mocked by MSW (`web/src/test/mocks/handlers.ts` doubles
  as living API contract documentation): tickets paging, problem+json error
  toasts, SSE-pushed notifications, the /admin permission guard, the
  approvals decision dialog, the profile page (OAuth identities, the TOTP
  lifecycle, API keys), every public auth flow (register, reset, verify,
  invitation), and admin user CRUD with the roles dialog.
- A contract gate (`src/test/mocks/contract.test.ts`) checks every MSW
  handler's path+method against `api/openapi/openapi.yaml` (`:param`
  normalized to `{param}`) — a drifted mock fails `make web-test`, the
  same drift philosophy as the codegen job. (Known SPA-side observation,
  outside this gate: `/api/streams/*` and `/api/users/{id}/enrich-demo`
  are called by the SPA but absent from the spec.)
- Browser e2e drives the real Go binary — the SPA is built, embedded, and
  served from the in-memory store with the demo seed — over critical
  journeys: register/login/logout, deep-link redirect-back with table
  state, tickets, the full approval chain across two roles (requester
  files and submits, the approver decides, the requester sees the
  outcome), admin user CRUD, and the landing/SPA serving split
  (`make web-test-e2e`; Playwright, Chromium).
- `e2e/a11y.spec.ts` axe-scans the rendered pages for serious/critical
  WCAG 2.x A/AA violations. Known color-contrast debt (muted-foreground
  tokens below 4.5:1) is allowlisted per page in the spec with a
  token-level fix owed — remove entries there as it gets fixed.
- `e2e/visual.spec.ts` snapshots the two static pages (landing, login)
  in light and vscode-dark. Baselines are Linux-Chromium PNGs under
  `web/e2e/visual.spec.ts-snapshots/`; the spec self-skips on other
  platforms. To regenerate: start the server
  (`go build -o web/e2e/.tmp/api-server ./cmd/api` and run it with
  `APP_DATABASE_URL= APP_DEMO_SEED=1 APP_DEMO_EMAIL=… APP_DEMO_PASSWORD=…
  APP_HTTP_ADDR=127.0.0.1:8090`), then
  `docker run --rm -v "$PWD/web":/work -w /work
  -e E2E_BASE_URL=http://host.docker.internal:8090
  --add-host=host.docker.internal:host-gateway
  mcr.microsoft.com/playwright:v1.63.0 npx playwright test
  e2e/visual.spec.ts --update-snapshots`.
- i18n: `src/lib/i18n.test.tsx` keeps the en/ru dictionaries in key
  parity, covers the en fallback, and the locale switch (translation,
  persistence, `<html lang>`).
- `make web-coverage` enforces the vitest floor (`vite.config.ts`
  `coverage.thresholds`, currently 62/54/54/63, baseline ~64.6/56.7/56.2/65.7).
  Same ratchet policy as the Go floor: raise it as coverage grows, never
  lower it. The admin pages remain the largest uncovered area.

CI note: workflow runs are temporarily skipped — every recent commit is
tagged `[skip ci]`. When re-enabling, push (or re-run) a commit without
the tag and expect the first `web-e2e` run to need attention: if the
visual specs diff on the runner, regenerate the baselines there (font
packages differ between the container and ubuntu-latest).

Rules:

- Feature completion requires tests listed in `docs/test-matrix.md`.
- Integration tests must not require a manually started stack; e2e tests must skip, not fail, when the stack is absent.
- New load-bearing code paths arrive with at least one table-driven test.
