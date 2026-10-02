import { expect, test } from "@playwright/test";

/**
 * Visual regression over the two static, seeded pages: the prerendered
 * landing and the login card — no live data, SSE clocks, or user content
 * that would make pixel comparisons flaky. Themes ride next-themes'
 * localStorage key, applied as the class on <html>.
 *
 * Baselines are generated on Linux (the CI platform) inside the
 * mcr.microsoft.com/playwright container against a server started on the
 * host — see docs/testing.md. On other platforms the spec self-skips:
 * font rasterization differs across operating systems, so only
 * like-for-like comparisons are meaningful.
 */
test.skip(process.platform !== "linux", "visual baselines are Linux-Chromium only");

const THEMES = ["light", "vscode-dark"] as const;

test.describe("visual regression", () => {
  for (const theme of THEMES) {
    test(`landing matches the ${theme} baseline`, async ({ page }) => {
      await page.emulateMedia({ reducedMotion: "reduce" });
      await page.addInitScript((value) => localStorage.setItem("theme", value), theme);
      await page.goto("/");
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
      expect(await page.screenshot({ fullPage: true })).toMatchSnapshot(`landing-${theme}.png`, {
        maxDiffPixelRatio: 0.02,
      });
    });

    test(`login matches the ${theme} baseline`, async ({ page }) => {
      await page.emulateMedia({ reducedMotion: "reduce" });
      await page.addInitScript((value) => localStorage.setItem("theme", value), theme);
      await page.goto("/login");
      await expect(page.locator("form").getByRole("button", { name: "Sign in" })).toBeVisible();
      expect(await page.screenshot()).toMatchSnapshot(`login-${theme}.png`, {
        maxDiffPixelRatio: 0.02,
      });
    });
  }
});
