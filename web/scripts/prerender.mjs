// Prerender pipeline for the landing page, run after `vite build`:
//
//   1. build the landing through Vite's SSR pipeline (react-dom/server),
//   2. inject the rendered markup into dist/landing/index.html, so the
//      embedded file is complete HTML before any JavaScript loads,
//   3. emit robots.txt + sitemap.xml from the same VITE_SITE_URL the page's
//      canonical tags use,
//   4. precompress every text asset (brotli + gzip), which the Go server
//      serves by Accept-Encoding without spending CPU per request.
//
// No browser (Puppeteer/Chromium) is involved: the page is deterministic
// static content, so renderToString is exact.
import fs from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { brotliCompressSync, constants as zlibConstants, gzipSync } from "node:zlib";
import { build } from "vite";

const root = path.resolve(import.meta.dirname, "..");
const dist = path.join(root, "dist");
const ssrOut = path.join(root, "dist-ssr");
const siteUrl = (process.env.VITE_SITE_URL ?? "https://go-template.dev").replace(/\/+$/, "");

// 1. SSR bundle of the landing entry (react-dom/server stays out of the
// browser bundle; Vite externalizes react for the node runtime here).
await build({
  root,
  configFile: path.join(root, "vite.config.ts"),
  logLevel: "warn",
  build: {
    ssr: path.join(root, "landing/entry-server.tsx"),
    outDir: "dist-ssr",
    emptyOutDir: true,
  },
});

const ssrBundle = fs
  .readdirSync(ssrOut)
  .filter((name) => name.endsWith(".js"))
  .map((name) => path.join(ssrOut, name))
  .toSorted()
  .at(0);
if (!ssrBundle) {
  throw new Error("SSR build produced no bundle");
}
const { render } = await import(pathToFileURL(ssrBundle).href);

// 2. Inject the rendered markup into the built landing HTML. The client
// build left <div id="root"></div> as the anchor; replacing it means the
// served file is complete HTML — crawlers never execute JavaScript.
// %VITE_SITE_URL% placeholders in the head (canonical/OG) are replaced here
// as well: this script is the single source for the site URL, so the build
// behaves identically with or without a .env file (CI sets the env var).
const landingPath = path.join(dist, "landing", "index.html");
let html = fs.readFileSync(landingPath, "utf8");
const anchor = '<div id="root"></div>';
if (!html.includes(anchor)) {
  throw new Error("landing template lost its #root anchor");
}
html = html.replace(anchor, `<div id="root">${render()}</div>`);
html = html.replaceAll("%VITE_SITE_URL%", siteUrl);
fs.writeFileSync(landingPath, html);

// 3. robots.txt + sitemap.xml, derived from the canonical URL so there is
// exactly one place (VITE_SITE_URL) to point at the real domain.
const disallowed = [
  "/api/",
  "/admin",
  "/dashboard",
  "/tickets",
  "/approvals",
  "/notifications",
  "/profile",
  "/login",
  "/register",
  "/forgot-password",
  "/reset-password",
  "/verify-email",
  "/accept-invitation",
  "/docs",
  "/openapi.yaml",
  "/health/",
  "/metrics",
  "/version",
];
fs.writeFileSync(
  path.join(dist, "robots.txt"),
  [
    "User-agent: *",
    "Allow: /",
    ...disallowed.map((p) => `Disallow: ${p}`),
    "",
    `Sitemap: ${siteUrl}/sitemap.xml`,
    "",
  ].join("\n"),
);
fs.writeFileSync(
  path.join(dist, "sitemap.xml"),
  [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">',
    "  <url>",
    `    <loc>${siteUrl}/</loc>`,
    "    <changefreq>monthly</changefreq>",
    "    <priority>1.0</priority>",
    "  </url>",
    "</urlset>",
    "",
  ].join("\n"),
);

// 4. Precompress text assets for the Go server's Content-Encoding
// negotiation. Binaries (png, woff, …) are already compressed.
const COMPRESSIBLE = new Set([
  ".html",
  ".js",
  ".css",
  ".svg",
  ".json",
  ".xml",
  ".txt",
  ".map",
  ".webmanifest",
]);
const MIN_SIZE = 1024;
let compressed = 0;
for (const entry of fs.readdirSync(dist, { recursive: true, withFileTypes: true })) {
  if (!entry.isFile()) continue;
  const full = path.join(entry.parentPath ?? dist, entry.name);
  if (COMPRESSIBLE.has(path.extname(full)) && fs.statSync(full).size >= MIN_SIZE) {
    const data = fs.readFileSync(full);
    fs.writeFileSync(
      `${full}.br`,
      brotliCompressSync(data, { params: { [zlibConstants.BROTLI_PARAM_QUALITY]: 11 } }),
    );
    fs.writeFileSync(`${full}.gz`, gzipSync(data, { level: 9 }));
    compressed++;
  }
}

fs.rmSync(ssrOut, { recursive: true, force: true });

// vite build emptied dist (emptyOutDir) — restore the committed placeholder
// so the go:embed directory keeps its checked-in anchor.
fs.closeSync(fs.openSync(path.join(dist, ".gitkeep"), "a"));
// oxlint-disable-next-line no-console -- the build pipeline's one status line
console.log(
  `prerender: landing injected, robots+sitemap written, ${compressed} assets precompressed`,
);
