import { expect, test } from "@playwright/test";

/**
 * The two entries of the embedded build: the prerendered landing owns "/"
 * and the SPA serves every other route — the same split
 * internal/httpapp/landing.go and spa.go enforce in production.
 */
test.describe("serving split", () => {
  test("serves the landing page at the root", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    // The landing repeats the links in the banner and the footer.
    await expect(page.getByRole("link", { name: "Sign in" }).first()).toHaveAttribute(
      "href",
      "/login",
    );
    await expect(page.getByRole("link", { name: "Get started" }).first()).toHaveAttribute(
      "href",
      "/register",
    );
  });

  test("serves the SPA for app routes", async ({ page }) => {
    await page.goto("/login");
    // Demo mode renders picker rows that also say "Sign in".
    await expect(page.locator("form").getByRole("button", { name: "Sign in" })).toBeVisible();
    await expect(page).toHaveTitle(/Go Template/i);
  });
});
