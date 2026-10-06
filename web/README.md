# web/ — React SPA frontend

A React 19 + Vite single-page app served by the Go API binary. Stack: TanStack
Router (file-based routes) + TanStack Query for data fetching, Tailwind CSS v4
with shadcn/ui components, generated typed API client from `api/openapi/openapi.yaml`.

## The layered-architecture contract

The slice-based dependency rules (features must not import each other, shared
must not import features, etc.) live in **[AGENTS.md](AGENTS.md)** and are
enforced by dependency-cruiser — run `make web-check-boundaries` to fail the
build on violations. Read it before adding code to `src/`.

## Contents

| Path | What it is |
| --- | --- |
| `src/routes/` | TanStack Router file-based routes (route tree generated into `src/routeTree.gen.ts`) |
| `src/features/<slice>/` | Vertical feature slices (`auth`, `tickets`, `admin-*`, ...) — UI + queries per domain |
| `src/shared/` | Cross-slice code: generated API client (`api/`), components, hooks, lib, types |
| `src/app/` | App shell, router setup, navigation, client/host layout |
| `landing/` | Self-contained landing page, prerendered to static HTML for SEO |
| `e2e/` | Playwright end-to-end specs (auth, tickets, a11y, visual snapshots) |
| `scripts/prerender.mjs` | Renders the landing page to static HTML during `npm run build` |
| `embed.go` | `go:embed all:dist` — the built SPA embedded into the Go binary |

## Embedded into the Go binary

Build output lands in `web/dist`, which the Go API embeds via `go:embed`
(`embed.go`) and serves directly. **Run `make web-build` before `go build`** or
the binary ships the committed placeholder assets.

## Commands

| npm | make | Does |
| --- | --- | --- |
| `npm run dev` | `make web-dev` | Vite dev server with HMR |
| `npm run gen:api` | — | Regenerate typed API client from the OpenAPI spec |
| `npm run build` | `make web-build` | Typecheck + Vite build + landing prerender into `dist/` |
| `npm run lint` | `make web-lint` | oxlint |
| `npm test` | `make web-test` | Vitest unit tests |
| `npm run test:e2e` | `make web-test-e2e` | Playwright e2e suite |

## Docs

- Development loop: [../docs/getting-started/development-workflow.md](../docs/getting-started/development-workflow.md)
- Frontend guide (slices, data fetching, conventions): [../docs/guides/frontend.md](../docs/guides/frontend.md)
