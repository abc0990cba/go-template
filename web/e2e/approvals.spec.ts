import { expect, test, type Page } from "@playwright/test";
import { approver, login, logout, requester } from "./helpers";

/**
 * The full approval chain over seeded demo data: the requester files an
 * Access request (a single-step chain decided by the "approver" role),
 * submits it, the approver finds it in the inbox and approves it, and the
 * requester sees the outcome.
 */

const title = `E2E standing desk access ${Date.now()}`;

/** Files an Access request as the requester and submits it for review. */
async function fileAndSubmitTicket(page: Page) {
  await login(page, requester.email, requester.password);
  await page.goto("/tickets");

  await page.getByRole("button", { name: "New ticket" }).click();
  await page.getByLabel("Ticket type").click();
  await page.getByRole("option", { name: /Access request/ }).click();
  await page.getByLabel("Title").fill(title);
  await page.getByRole("button", { name: "Create draft" }).click();
  await expect(page.getByText(/created as a draft/)).toBeVisible({ timeout: 15_000 });

  // Drafts leave the chain only once submitted for review.
  await page.getByRole("link", { name: title }).click();
  await expect(page).toHaveURL(/\/tickets\/.+/, { timeout: 15_000 });
  await page.getByRole("button", { name: "Submit for approval" }).click();
  await expect(page.getByText(/in review/i).first()).toBeVisible({ timeout: 15_000 });
  await logout(page);
}

test.describe("approvals journey", () => {
  test("requester files, approver decides, requester sees the outcome", async ({ page }) => {
    await fileAndSubmitTicket(page);

    // The approver's inbox is role-scoped server-side.
    await login(page, approver.email, approver.password);
    await page.goto("/approvals");
    const row = page.getByRole("row", { name: new RegExp(title.replace(/[()]/g, "\\$&")) });
    await expect(row).toBeVisible({ timeout: 15_000 });

    await row.getByRole("button", { name: "Approve" }).click();
    await expect(page.getByRole("heading", { name: /Approve TCK-/ })).toBeVisible();
    await page.getByLabel("Comment (optional)").fill("Desk access makes sense");
    await page.getByRole("button", { name: "Approve ticket" }).click();
    await expect(page.getByText(/approved$/)).toBeVisible({ timeout: 15_000 });
    await logout(page);

    // The requester's list reflects the decision.
    await login(page, requester.email, requester.password);
    await page.goto("/tickets");
    const decidedRow = page.getByRole("row", { name: new RegExp(title.replace(/[()]/g, "\\$&")) });
    await expect(decidedRow).toBeVisible({ timeout: 15_000 });
    await expect(decidedRow.getByText("Approved")).toBeVisible();
  });
});
