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
| `src/shared/` | transport (`api/core`, `api/pagination`), the generated contract (`api/schema.gen.ts`), core types, guards, i18n, hooks, generic components (`ui/`, `data-table`), test infra | — (leaf) | anything in `features/`, `app/`, `routes/` |

## The contract is generated — never hand-write wire shapes

`api/openapi/openapi.yaml` is the single source of truth for every wire
shape and endpoint.

1. `npm run gen:api` generates `src/shared/api/schema.gen.ts`
   (openapi-typescript; the CI `codegen` job fails if it is stale).
2. `api.ts` calls the typed client — `api.GET/POST(...)` + `unwrap()` from
   `@/shared/api/core` — with exact spec path literals, typed query/body,
   and thrown `ApiError`. Raw `fetch` is forbidden.
3. `types.ts` derives every exported type from `components["schemas"]` /
   `operations[...]` of `schema.gen.ts`. To change a shape, change the spec
   and regenerate; hand-edited interfaces drift by construction.
4. Editing the spec → run `npm run gen:api` (or `make dev`, which watches
   it) and fix whatever `tsc` surfaces at the call sites.

## The rules (CI-enforced)

1. **Boundary direction** — `make web-check-boundaries` (dependency-cruiser).
   Cross-slice imports go through `features/<slice>/index.ts` only; a slice's
   internals are private.
2. **Size budget** — `make check-size-web`: ≤ **400 lines** per hand-written
   file (matches the Go rule), ≤ **80 lines** per route file, tests,
   `*.gen.ts`, and `shared/components/ui/` (shadcn) exempt. Over budget =
   extract a component/dialog into a sibling file.
3. **No logic in route files** — a route validates search params and renders
   a feature page; queries, mutations, and markup live in the slice.
4. **State model** — server state is TanStack Query (never mirrored into
   local state); table/filter state lives in the URL via search schemas;
   `zod` schemas sit next to the form they validate (`schemas.ts`).

## New-feature checklist

1. `src/features/<slice>/` with `api.ts` (typed client calls), `types.ts`
   (contract derivations), `schemas.ts` (zod), page components, `index.ts`
   exporting **only** what other layers need.
2. Thin route file importing the page + search schema.
3. MSW default handler block in `src/test/mocks/handlers/<slice>.ts` — the
   contract test (`src/test/mocks/contract.test.ts`) fails when a mock's
   path+method is missing from the spec **or** its fixture no longer
   matches the operation's declared response schema.
4. Tests colocated next to the pages they cover.
5. Run: `make web-lint web-test web-check-boundaries check-size-web`.

## Forbidden

- Importing `features/<x>/<file>` deep paths (only `features/<x>` index).
- `shared` reaching upward; features importing `app`/`routes`.
- New files over the budget; logic in `routes/`.
- Adding a state-management library; duplicating server data into local
  state; bypassing the typed client with raw `fetch`.
- Hand-writing wire-shape interfaces or editing `schema.gen.ts`,
  `routeTree.gen.ts`, or the baselines under `*-snapshots/` by hand.

## Verification commands

```sh
make web-lint web-test web-coverage   # lint, vitest, coverage floor
make check-size-web web-check-boundaries
npm run build && npm run test:e2e     # tsc + vite + prerender; Playwright
```
