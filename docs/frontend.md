# Frontend

The UI is a React single-page admin console in `web/`, embedded into the Go binary at build time. One artifact serves both the JSON API and the SPA.

## Stack

- **Vite 7 + React 19 + TypeScript** (strict: `strict`, `noUncheckedIndexedAccess`, `verbatimModuleSyntax`, …)
- **Tailwind CSS v4** with **shadcn/ui** (new-york style, neutral base, lucide icons). Ready-made admin pieces come straight from the official shadcn set: collapsible Sidebar, login blocks, Chart (Recharts), Data Table, Command palette, Alert Dialog, Sheet, Sonner toasts.
- **TanStack Router** — file-based routes in `src/routes`, type-safe search params. Table state (pagination mode, page/cursor, filters) lives in the URL, so any table view is linkable and back-button friendly. Auth guards run in `beforeLoad` of the `_auth` layout.
- **TanStack Query** — all server state. Request deduplication, caching, and invalidation happen here; the SSE stream invalidates queries as push events arrive.
- **TanStack Table** — data tables (sorting client-side; pagination is delegated to the server).
- **react-hook-form + zod** for forms; **next-themes** for dark mode.
- **oxlint** (correctness/suspicious at error; pedantic/restriction/style as warnings; `no-console`, non-null assertions and array-reduce at error) and **oxfmt** for formatting. Generated code (`src/routeTree.gen.ts`, `src/components/ui/**`, the shadcn `use-mobile` hook) is lint-exempt.

## Layout

```
web/
  src/routes/          file-based routes; _auth.tsx is the authenticated layout
  src/components/ui/   shadcn components (generated; do not hand-edit)
  src/components/      app shell, explainer banner, KPI card, data-table kit
  src/hooks/           use-event-stream (SSE), use-paged-list, use-mobile
  src/lib/             api client, query client + router, auth guard, search schemas, types
  dist/                build output, embedded by the Go binary (web/embed.go)
```

## Server contract

- Dev: `make web-dev` starts Vite on :5173 and proxies `/api`, `/health`, `/docs`, `/openapi.yaml` to :8080 — same origin, so the session cookie just works and no CORS is needed.
- Prod: `make web-build` fills `web/dist`; `go:embed` (`web/embed.go`) bakes it into `cmd/api`, which serves assets with an `index.html` fallback for client-side routes.
- Auth: `POST /api/auth/login` sets the `app_session` cookie; `GET /api/me` is the session probe. The old HTML form + double-submit CSRF flow is gone; unsafe `/api` methods are guarded by an Origin/Sec-Fetch-Site check plus `SameSite=Lax`.
- Live updates: one `EventSource('/api/events/stream')` per session (mounted in `LiveProvider`) carries `analytics.event`, `job.updated`, and periodic `snapshot` messages; the hook throttles query invalidation.

## Conventions

- Every page mounts an `ExplainerBanner` ("what this is / how to use it / under the hood") under its header.
- Every list page uses the shared `DataTable` + `ModeSwitcher` (offset | cursor | keyset) and `usePagedList`; per-mode controls make the pagination trade-offs visible.
- API access goes through `src/lib/api.ts`; wire types live in `src/lib/types.ts` and mirror `api/openapi/openapi.yaml`.

## Checks

```
make web-lint   # oxlint + oxfmt --check
make web-build  # tsc --noEmit + vite build
make web-test   # vitest
```
