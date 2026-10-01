import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { LandingPage } from "./LandingPage";

/**
 * The landing must stay crawler-grade: a single h1, real text content for
 * every section, and working links into the app. Rendering through
 * react-dom/server here mirrors exactly what scripts/prerender.mjs bakes
 * into dist/landing/index.html.
 */
describe("LandingPage", () => {
  const html = renderToString(<LandingPage />);

  it("has exactly one h1 as the LCP heading", () => {
    expect(html.match(/<h1/g)).toHaveLength(1);
    expect(html).toContain("The Go + React platform that ships as");
  });

  it("renders the section headings and FAQ answers as DOM text", () => {
    expect(html).toContain("Everything an internal product needs, on day one");
    expect(html).toContain("Frequently asked questions");
    expect(html).toContain("One static Go binary plus Postgres is enough");
    expect(html.match(/<details/g)).toHaveLength(5);
  });

  it("links visitors into the app", () => {
    expect(html).toContain('href="/register"');
    expect(html).toContain('href="/login"');
  });

  it("keeps the markup deterministic for hydration", () => {
    // Two renders must be byte-identical — a mismatch would make
    // hydrateRoot warn and re-render in the browser.
    expect(renderToString(<LandingPage />)).toBe(html);
  });
});
