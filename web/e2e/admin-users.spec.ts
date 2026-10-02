import { expect, test } from "@playwright/test";
import { admin, login } from "./helpers";

/**
 * Admin user management over the real backend: create, find via
 * server-side search, edit, delete — the same flow an operator walks.
 */
test.describe("admin users journey", () => {
  test("creates, finds, edits, and deletes a user", async ({ page }) => {
    await login(page, admin.email, admin.password);
    await page.goto("/admin/users");
    await expect(page.getByRole("heading", { name: "Users", exact: true })).toBeVisible();

    const email = `e2e-crud-${Date.now()}@example.com`;
    await page.getByRole("button", { name: "New user" }).click();
    await page.getByLabel("Name").fill("Crud Walker");
    await page.getByLabel("Email").fill(email);
    await page.getByRole("button", { name: "Create" }).click();
    await expect(page.getByText(`Created ${email}`)).toBeVisible({ timeout: 15_000 });

    // Server-side search commits on Enter.
    const search = page.getByPlaceholder("Search name or email, then Enter");
    await search.fill(email);
    await search.press("Enter");
    const row = page.getByRole("row", { name: new RegExp(email.replace(/[.@]/g, "\\$&")) });
    await expect(row).toBeVisible({ timeout: 15_000 });

    await row.getByLabel(`Edit ${email}`).click();
    await expect(page.getByRole("heading", { name: "Edit user" })).toBeVisible();
    await page.getByLabel("Name").fill("Crud Walker II");
    await page.getByRole("button", { name: "Save" }).click();
    await expect(page.getByText("User updated")).toBeVisible({ timeout: 15_000 });

    await row.getByLabel(`Delete ${email}`).click();
    await expect(page.getByText(`Delete ${email}?`)).toBeVisible();
    await page.getByRole("button", { name: "Delete", exact: true }).click();
    await expect(page.getByText("User deleted")).toBeVisible({ timeout: 15_000 });

    await search.fill(email);
    await search.press("Enter");
    await expect(page.getByText(`No users match “${email}”.`)).toBeVisible({
      timeout: 15_000,
    });
  });
});
