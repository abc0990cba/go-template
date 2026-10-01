# ADR 0010 — Embed the SPA into the api Binary

**Status:** Accepted

## Context

The template ships a React SPA and a Go API that are deployed together.
Two artifacts mean CDN configuration, CORS, version skew between frontend
and API, and an extra deploy step — all noise for a product whose UI is
same-origin by design.

## Decision

`web/embed.go` declares `//go:embed all:dist`; the Dockerfile builds the
SPA first and copies `dist/` into the Go build context; the api serves it
with an SPA-aware fallback (`serveSPA`): extension-less unknown paths →
`index.html` (written directly to dodge the stdlib redirect), asset-like
paths → 404, everything else via `FileFromFS`, all with
`Cache-Control: no-cache` (no asset hashing yet, so no aggressive
caching). A placeholder dist produces a clean 503 with build
instructions.

Dev mode uses Vite's proxy (`:5173 → :8080`) with an explicit `Origin`
rewrite so the CSRF middleware sees the same-origin requests it will see
in production.

## Consequences

**Positive:** one artifact, one port, zero CORS, atomic frontend+API
releases; `go test ./...` covers the serving path; the demo boots with
`docker run` alone.

**Negative:** any frontend change rebuilds the backend image (correct for
this coupling level, wrong for a large frontend org); no CDN — large
asset payloads ride the app origin (add hashing + immutable caching when
that matters); the `no-cache` on hashed assets is deliberately
conservative until hashing lands.

## Alternatives considered

- *Separate static hosting + CDN* — the right call at frontend-team
  scale; the API/SPA split point (pure `/api` JSON) makes that migration
  a config change, not a rewrite.
- *HTMX/server templates* — a different product philosophy; the SPA was
  chosen to exercise the modern TS toolchain (TanStack Router/Query,
  shadcn) that adopters expect.

**Update (0026):** assets are content-hashed now; serving gained immutable
caching for `assets/*`, ETag revalidation, brotli/gzip precompression, and
a prerendered landing at `/` — see [ADR 0026](0026-landing-prerender.md).
