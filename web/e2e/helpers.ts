import { expect, type Page } from "@playwright/test";

/**
 * Shared E2E fixtures mirroring what the server seeds: the superadmin
 * (APP_DEMO_EMAIL, bootstrap.SeedDemoUser) and the demo dataset accounts
 * (APP_DEMO_SEED=1, lib/demo-accounts.ts mirrors internal demodata.go).
 */
export const admin = {
  email: process.env.E2E_ADMIN_EMAIL ?? "e2e-admin@example.com",
  password: process.env.E2E_ADMIN_PASSWORD ?? "e2e-admin-password-123",
};

export const requester = {
  email: "demo-u-0004@demo.example",
  password: "password123",
  name: "Rita Requester",
};

/** Holds the "approver" role: decides the final step of a chain. */
export const approver = {
  email: "demo-u-0005@demo.example",
  password: "password123",
  name: "Ari Approver",
};

/** Drives the login form and waits for the post-login landing page. */
export async function login(page: Page, email: string, password: string) {
  await page.goto("/login");
  await page.getByPlaceholder("you@example.com").fill(email);
  await page.getByPlaceholder("••••••••").fill(password);
  // Demo mode (APP_DEMO_SEED=1) renders the demo picker beside the form;
  // its rows carry their own "Sign in" buttons, so scope to the form.
  await page.locator("form").getByRole("button", { name: "Sign in" }).click();
  // The session probe resolves before any route renders, so the shell
  // (any heading inside the app) marks "signed in".
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible({ timeout: 15_000 });
}

/** Signs out through the account menu of either shell. */
export async function logout(page: Page) {
  await page.getByRole("button", { name: "Account menu" }).click();
  await page.getByRole("menuitem", { name: "Log out" }).click();
  // Demo mode renders picker rows that also say "Sign in"; scope to the form.
  await expect(page.locator("form").getByRole("button", { name: "Sign in" })).toBeVisible({
    timeout: 15_000,
  });
}
