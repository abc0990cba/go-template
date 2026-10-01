# ADR 0026 — Prerender the Landing Page and Fix Static Serving Caching

**Status:** Accepted

## Context

The public landing page at `/` needs modern SEO (crawler-ready HTML,
canonical/OG tags, structured data, sitemap) and green Core Web Vitals.
The SPA cannot provide either: crawlers receive an empty `<div id="root">`,
every asset travels with `Cache-Control: no-cache` (deliberate while assets
were unhashed, per ADR 0010), and nothing is compressed. Runtime SSR
(a Node server rendering per request, proxied behind Go) was considered and
rejected: the landing is static copy, so per-request rendering adds a Node
runtime to production and latency without improving any crawler-visible
byte.

## Decision

Render the landing **once at build time** and keep serving everything from
the embedded filesystem:

- `web/landing/` is a second Vite entry (own JS/CSS chunks, no TanStack
  Router, no admin bundle). `npm run build` = `tsc` → `vite build` →
  `scripts/prerender.mjs`.
- The prerender script builds the page through Vite's SSR pipeline
  (`react-dom/server`), injects the markup into the landing HTML template,
  emits `robots.txt` + `sitemap.xml` from `VITE_SITE_URL`, and precompresses
  every text asset to `.br`/`.gz` siblings.
- `internal/httpapp/landing.go` serves the prerendered HTML at `/`
  (registered before the SPA fallback, so only the exact root path is
  intercepted); `serveSPA` keeps every other extension-less path.
- `serveEmbedded` gives all embedded files strong ETags with 304
  revalidation, `Cache-Control: public, max-age=31536000, immutable` for
  content-hashed `assets/*`, `no-cache` for HTML/robots/sitemap, and
  `Content-Encoding: br|gzip` negotiation against the precompressed
  siblings — zero runtime compression CPU.
- Vite now empties `dist` per build (`emptyOutDir`): hashed filenames made
  old output dead weight that accumulated in the binary forever; the
  committed `dist/.gitkeep` placeholder is re-created by the prerender
  script.

CSP is unchanged: the page references external hashed scripts only, and
JSON-LD blocks are data, not executable script (`script-src 'self'` holds).

## Consequences

**Positive:** the landing is complete HTML on the first byte (best possible
LCP/TTFB for a single binary); crawlers and link previews get copy, meta,
and structured data without executing JavaScript; production gains brotli
and year-long asset caching for the whole SPA; still one distroless image
with no Node runtime.

**Negative:** content changes require a rebuild (fine for static copy);
`react-dom/server` runs in CI, not in the browser bundle; hydration adds
~66 KB gzip of react-dom to the landing (the price of keeping the page in
the React stack — drop the hydration entry if that ever matters); the
landing HTML template and the FAQPage JSON-LD must be kept in sync by hand.

## Alternatives considered

- *Runtime SSR behind the Go proxy (Vite SSR / TanStack Start)* — the
  general answer for per-request dynamic pages; overhead with no payoff for
  static copy, and a second runtime to operate.
- *Next.js or Astro for the landing* — Astro's zero-JS islands are the
  stronger fit if a multi-page marketing site (blog, docs) grows here; both
  emit plain static files, so migrating later leaves the Go serving side
  untouched. A second toolchain was not worth it for one page.
- *Go html/template landing* — fastest possible bytes, but abandons React
  and splits the page's markup away from the component toolchain.
