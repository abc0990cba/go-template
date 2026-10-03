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

test.describe("landing interactions", () => {
  test("opens a feature dialog and closes it with Escape", async ({ page }) => {
    await page.goto("/");
    // The native <dialog> ships in the static HTML (closed); the button
    // upgrade opens it with showModal().
    await page.getByRole("button", { name: "Authentication & MFA" }).click();
    const dialog = page.getByRole("dialog", { name: "Authentication & MFA" });
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("mfa.required");
    await page.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
  });

  test("switches the interface language and remembers it", async ({ page }) => {
    await page.goto("/");
    // The switcher appears in the header and the footer; use the header one.
    await page.getByRole("banner").getByRole("button", { name: "Русский" }).click();
    await expect(page.locator("html")).toHaveAttribute("lang", "ru");
    await expect(page.getByRole("heading", { level: 1 })).toContainText("один бинарник");
    // The stored choice survives a reload.
    await page.reload();
    await expect(page.locator("html")).toHaveAttribute("lang", "ru");
  });
});
