import { expect, test } from "@playwright/test";
import { login, requester } from "./helpers";

/**
 * The requester journey over seeded demo data: the tickets list renders the
 * seeded rows, and the create dialog files a real ticket through the API,
 * which comes back as a toast and a new row.
 */
test.describe("tickets journey", () => {
  test("lists seeded tickets with their references", async ({ page }) => {
    await login(page, requester.email, requester.password);
    await page.goto("/tickets");

    await expect(page.getByRole("heading", { name: "My Tickets" })).toBeVisible();
    // demodata seeds tickets across the whole status range.
    await expect(page.getByText(/TCK-\d{4}/).first()).toBeVisible({ timeout: 15_000 });
  });

  test("files a ticket through the create dialog", async ({ page }) => {
    const title = `E2E chair request ${Date.now()}`;
    await login(page, requester.email, requester.password);
    await page.goto("/tickets");

    await page.getByRole("button", { name: "New ticket" }).click();
    await page.getByLabel("Ticket type").click();
    // The seeded types carry their approval-chain length in the label.
    await page.getByRole("option").first().click();
    await page.getByLabel("Title").fill(title);
    await page.getByRole("button", { name: "Create draft" }).click();

    await expect(page.getByText(/created as a draft/)).toBeVisible({ timeout: 15_000 });
    await expect(page.getByRole("link", { name: title })).toBeVisible({ timeout: 15_000 });
  });

  test("opens a ticket detail from the list", async ({ page }) => {
    await login(page, requester.email, requester.password);
    await page.goto("/tickets");

    // Rows link their TITLE to the detail page; the TCK reference sits in
    // the number cell beside it.
    const firstTicket = page.locator('a[href^="/tickets/"]').first();
    await expect(firstTicket).toBeVisible({ timeout: 15_000 });
    await firstTicket.click();

    // The detail page shows the ticket's title heading.
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible({ timeout: 15_000 });
    expect(page.url()).toMatch(/\/tickets\/.+/);
  });
});
