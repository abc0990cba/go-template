import { expect, test } from "@playwright/test";
import { admin, login, logout } from "./helpers";

/**
 * The identity journey over the real backend: register auto-logs-in, the
 * account menu signs out, a wrong password surfaces the problem detail,
 * and the redirect-back returns you to where you were heading.
 */
test.describe("auth journey", () => {
  test("registers a new account and lands on the dashboard", async ({ page }) => {
    await page.goto("/register");
    const email = `e2e-${Date.now()}@example.com`;
    await page.getByLabel("Name").fill("E2E Walker");
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Password").fill("walking-password-1");
    await page.getByRole("button", { name: "Create account" }).click();

    // Registration issues the session cookie right away and navigates.
    await expect(page.getByRole("heading", { name: /Welcome, E2E/ })).toBeVisible({
      timeout: 15_000,
    });
  });

  test("signs out from the account menu and reaches the login card", async ({ page }) => {
    await login(page, admin.email, admin.password);
    await logout(page);
    expect(page.url()).toContain("/login");
  });

  test("rejects a wrong password with the problem detail", async ({ page }) => {
    await page.goto("/login");
    await page.getByPlaceholder("you@example.com").fill(admin.email);
    await page.getByPlaceholder("••••••••").fill("definitely-wrong");
    await page.locator("form").getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByText("invalid credentials")).toBeVisible({
      timeout: 10_000,
    });
  });

  test("returns to the saved location after signing in", async ({ page }) => {
    // Heading somewhere guarded while anonymous lands on /login?redirect=…
    // (/profile keeps no search params; list pages carry their table state
    // in the URL, which the login card deliberately refuses to redirect to.)
    await page.goto("/profile");
    await expect(page).toHaveURL(/\/login/);
    await page.getByPlaceholder("you@example.com").fill(admin.email);
    await page.getByPlaceholder("••••••••").fill(admin.password);
    await page.locator("form").getByRole("button", { name: "Sign in" }).click();
    await expect(page).toHaveURL(/\/profile/, { timeout: 15_000 });
    await expect(page.getByRole("heading", { name: "Profile" })).toBeVisible();
  });
});
