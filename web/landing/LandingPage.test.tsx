import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { LandingPage } from "./LandingPage";
import { FEATURE_IDS } from "./feature-ids";

/**
 * The landing must stay crawler-grade: a single h1, real text content for
 * every section — including all 18 feature descriptions, which live inside
 * their (closed) dialogs — and working links into the app. Rendering
 * through react-dom/server here mirrors exactly what scripts/prerender.mjs
 * bakes into dist/landing/index.html.
 */
describe("LandingPage", () => {
  const html = renderToString(<LandingPage />);

  it("has exactly one h1 as the LCP heading", () => {
    expect(html.match(/<h1/g)).toHaveLength(1);
    expect(html).toContain("one binary");
  });

  it("renders the section headings as DOM text", () => {
    expect(html).toContain("Every feature a real product needs");
    expect(html).toContain("From clone to production in three steps");
    expect(html).toContain("Frequently asked questions");
  });

  it("keeps every feature description in the static DOM", () => {
    // One closed dialog per feature area — the full copy is crawlable even
    // though the interaction needs JavaScript.
    expect(html.match(/<dialog/g)).toHaveLength(FEATURE_IDS.length);
    expect(html).toContain("mfa.required");
    // react-dom escapes the apostrophe in the server render.
    expect(html).toContain("What&#x27;s inside");
  });

  it("renders the FAQ answers as DOM text", () => {
    expect(html).toContain("One static Go binary plus Postgres is enough");
    expect(html.match(/<details/g)).toHaveLength(6);
  });

  it("renders the language switcher with all four locales", () => {
    for (const code of ["en", "ru", "fr", "de"]) {
      expect(html).toContain(`>${code}<`);
    }
    expect(html).toContain("aria-pressed=");
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
