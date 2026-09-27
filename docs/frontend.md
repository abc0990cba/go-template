# Frontend

The UI is a React single-page admin console in `web/`, embedded into the Go binary at build time. One artifact serves both the JSON API and the SPA.

## Stack

- **Vite 7 + React 19 + TypeScript** (strict: `strict`, `noUncheckedIndexedAccess`, `verbatimModuleSyntax`, …)
- **Tailwind CSS v4** with **shadcn/ui** (new-york style, neutral base, lucide icons). Ready-made admin pieces come straight from the official shadcn set: collapsible Sidebar, login blocks, Chart (Recharts), Data Table, Command palette, Alert Dialog, Sheet, Sonner toasts.
- **TanStack Router** — file-based routes in `src/routes`, type-safe search params. Table state (pagination mode, page/cursor, filters) lives in the URL, so any table view is linkable and back-button friendly. Auth guards run in `beforeLoad`: `_client.tsx` guards the user-facing workspace, `admin.tsx` additionally requires the `admin_console:access` permission.
- **TanStack Query** — all server state. Request deduplication, caching, and invalidation happen here; the SSE stream invalidates queries as push events arrive.
- **TanStack Table** — data tables (sorting client-side; pagination is delegated to the server).
- **react-hook-form + zod** for forms; **next-themes** for theming. The theme picker offers `light`, `dark`, `vscode-light`, `vscode-dark`, `catppuccin-latte` ("Cappuccino Light"), and `catppuccin-mocha` ("Cappuccino Dark"); each value maps to a token block in `src/styles/app.css` and is applied as a class on `<html>`. Chart palettes (`--chart-1…5`) are defined per theme — dark themes carry lighter, high-chroma hues so lines and bars stay readable.
- **oxlint** (correctness/suspicious at error; pedantic/restriction/style as warnings; `no-console`, non-null assertions and array-reduce at error) and **oxfmt** for formatting. Generated code (`src/routeTree.gen.ts`, `src/components/ui/**`, the shadcn `use-mobile` hook) is lint-exempt.

## Layout

```
web/
  src/routes/          file-based routes; _client.tsx (workspace at /) and admin.tsx (/admin console) are the two authenticated layouts
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
- Every list page uses the shared `DataTable` + `PaginationBar` and `useTableView`: the strategy switcher (offset | cursor | keyset) and the view switcher (Pages | Infinite scroll) sit under every server-paginated table, and the mode/view/page/cursor state lives in the URL (`?mode=&view=&page=&after=`). Infinite scroll appends pages via `useInfiniteList` (IntersectionObserver sentinel + a manual "Load more" button). The audit page keeps bespoke row markup but mounts the same `PaginationBar`.
- API access goes through `src/lib/api.ts`; wire types live in `src/lib/types.ts` and mirror `api/openapi/openapi.yaml`.

## Checks

```
make web-lint   # oxlint + oxfmt --check
make web-build  # tsc --noEmit + vite build
make web-test   # vitest
```

## Client workspace vs admin console

The SPA is one codebase with two explicit areas, following the common
`/admin`-prefix + layout-guard pattern:

- `_client.tsx` — the pathless authenticated layout for the user-facing
  workspace at the root paths (`/`, `/tickets`, `/approvals`,
  `/notifications`, `/profile`). It renders `ClientShell` (top navbar,
  personal home). Any authenticated account lands here.
- `admin.tsx` — the `/admin` layout for the back office (`/admin/users`,
  `/admin/roles`, …). It renders the sidebar `AppShell` and its `beforeLoad`
  runs `ensurePermission("admin_console", "access")`; accounts without the
  permission are redirected to `/`. The server re-checks every admin API.
- Granting `admin_console:access` to a role (Roles page matrix) makes the
  console visible to it; the default grants cover admin, manager, and viewer.

To add a page: put `admin.foo.tsx` (route `/admin/foo`) or
`_client.foo.tsx` (route `/foo`) in `src/routes`, then register the nav item
in `app-shell.tsx` (`NAV_*`) or `client-shell.tsx` (`NAV_LINKS`).
