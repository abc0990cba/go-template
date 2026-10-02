# Frontend architecture contract (web/)

This codebase is **rigidly structured on purpose**: it is written with LLM
assist, and strict frames beat good intentions. The rules below are
enforced by CI (dependency-cruiser + size budgets), so a violation cannot
merge — but aim to get them right the first time.

## Layers (dependencies point one way only)

```
routes/   →  features/<slice>/  →  shared/
   ↑                ↑
   app/  ───────────┘
```

| Layer | Owns | May import | Must never import |
|---|---|---|---|
| `src/routes/` | URL wiring: `createFileRoute`, search validation, guards, `<FeaturePage search={…}/>` | `features/*/index.ts`, `shared` | feature internals, app internals |
| `src/app/` | composition: router instance, shells, header widgets | `features/*/index.ts`, `shared` | `src/routes/` (only `routeTree.gen.ts`) |
| `src/features/<slice>/` | everything domain-shaped: `api.ts`, `types.ts`, `schemas.ts`, pages, dialogs, tests | `shared`, **other slices only via their `index.ts`** | `app`, `routes`, other slices' internals |
| `src/shared/` | transport (`api/core`, `api/pagination`), core types, guards, i18n, hooks, generic components (`ui/`, `data-table`), test infra | — (leaf) | anything in `features/`, `app/`, `routes/` |

## The rules (CI-enforced)

1. **Boundary direction** — `make web-check-boundaries` (dependency-cruiser).
   Cross-slice imports go through `features/<slice>/index.ts` only; a slice's
   internals are private.
2. **Size budget** — `make check-size-web`: ≤ **400 lines** per hand-written
   file (matches the Go rule), ≤ **80 lines** per route file, tests and
   `shared/components/ui/` (shadcn) exempt. Over budget = extract a
   component/dialog into a sibling file.
3. **No logic in route files** — a route validates search params and renders
   a feature page; queries, mutations, and markup live in the slice.
4. **State model** — server state is TanStack Query (never mirrored into
   local state); table/filter state lives in the URL via search schemas;
   `zod` schemas sit next to the form they validate (`schemas.ts`).

## New-feature checklist

1. `src/features/<slice>/` with `api.ts` (endpoints via `shared/api/core`
   verbs), `types.ts`, `schemas.ts` (zod), page components, `index.ts`
   exporting **only** what other layers need.
2. Thin route file importing the page + search schema.
3. MSW default handler block in `src/shared/test/mocks/handlers/<slice>.ts`
   — the contract test
   (`src/test/mocks/contract.test.ts`) fails if a mock points outside
   `api/openapi/openapi.yaml`.
4. Tests colocated next to the pages they cover.
5. Run: `make web-lint web-test web-check-boundaries check-size-web`.

## Forbidden

- Importing `features/<x>/<file>` deep paths (only `features/<x>` index).
- `shared` reaching upward; features importing `app`/`routes`.
- New files over the budget; logic in `routes/`.
- Adding a state-management library; duplicating server data into local
  state; bypassing `shared/api/core` with raw `fetch`.
- Editing `routeTree.gen.ts` or the baselines under `*-snapshots/` by hand.

## Verification commands

```sh
make web-lint web-test web-coverage   # lint, vitest, coverage floor
make check-size-web web-check-boundaries
npm run build && npm run test:e2e     # tsc + vite + prerender; Playwright
```
