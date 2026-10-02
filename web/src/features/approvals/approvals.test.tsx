import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { decisionFixture, page, ticketFixture } from "@/test/mocks/handlers";
import { server } from "@/test/mocks/server";
import { renderRoute, seedMe } from "@/test/test-utils";

/**
 * Integration tests of the approvals inbox through the real route tree:
 * rendering, the approve/reject decision dialog against the actions API,
 * and the decided-history tab.
 */

const user = userEvent.setup();

function seedApprover() {
  return seedMe({ permissions: ["tickets:read", "tickets:approve", "tickets:read_all"] });
}

async function openApprovals() {
  renderRoute("/approvals");
  await screen.findByRole("heading", { name: "Approvals" }, { timeout: 4000 });
}

describe("approvals page", () => {
  it("lists the inbox with per-row Approve and Reject actions", async () => {
    seedApprover();
    server.use(
      http.get("/api/approvals", () =>
        HttpResponse.json(
          page([
            ticketFixture({ id: "t-1", number: 1, title: "VPN for the new hire" }),
            ticketFixture({ id: "t-2", number: 2, title: "Standing desk" }),
          ]),
        ),
      ),
    );

    await openApprovals();

    expect(await screen.findByText("VPN for the new hire")).toBeInTheDocument();
    expect(screen.getByText("Standing desk")).toBeInTheDocument();
    expect(screen.getByText(/Waiting for you \(2\)/)).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Approve" })).toHaveLength(2);
    expect(screen.getAllByRole("button", { name: "Reject" })).toHaveLength(2);
  });

  it("shows the empty state when nothing waits on the caller's roles", async () => {
    seedApprover();
    await openApprovals();
    expect(await screen.findByText("Nothing waits on your roles right now.")).toBeInTheDocument();
  });

  it("approves through the dialog and posts the action", async () => {
    seedApprover();
    let action: Record<string, unknown> | undefined;
    server.use(
      http.get("/api/approvals", () =>
        HttpResponse.json(page([ticketFixture({ id: "t-7", number: 7, title: "New monitor" })])),
      ),
      http.post("/api/tickets/:id/actions", async ({ request }) => {
        action = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(ticketFixture({ id: "t-7", number: 7, status: "in_review" }));
      }),
    );
    await openApprovals();

    await user.click(await screen.findByRole("button", { name: "Approve" }));
    expect(await screen.findByText("Approve TCK-0007")).toBeInTheDocument();
    // Approve's comment is optional; still recorded when present.
    await user.type(screen.getByLabelText("Comment (optional)"), "Looks good");
    await user.click(screen.getByRole("button", { name: "Approve ticket" }));

    await waitFor(() => expect(action).toEqual({ action: "approve", comment: "Looks good" }));
    expect(await screen.findByText("Ticket TCK-0007 approved")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("requires a comment to reject and posts it", async () => {
    seedApprover();
    let action: Record<string, unknown> | undefined;
    server.use(
      http.get("/api/approvals", () =>
        HttpResponse.json(page([ticketFixture({ id: "t-9", number: 9, title: "Fridge move" })])),
      ),
      http.post("/api/tickets/:id/actions", async ({ request }) => {
        action = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(ticketFixture({ id: "t-9", number: 9, status: "rejected" }));
      }),
    );
    await openApprovals();

    await user.click(await screen.findByRole("button", { name: "Reject" }));
    expect(await screen.findByText("Reject TCK-0009")).toBeInTheDocument();

    const confirm = screen.getByRole("button", { name: "Reject ticket" });
    expect(confirm).toBeDisabled();
    await user.type(screen.getByLabelText("Comment (required)"), "Wrong cost center");
    expect(confirm).toBeEnabled();
    await user.click(confirm);

    await waitFor(() => expect(action).toEqual({ action: "reject", comment: "Wrong cost center" }));
    expect(await screen.findByText("Ticket TCK-0009 rejected")).toBeInTheDocument();
  });

  it("lists past decisions under My decisions", async () => {
    seedApprover();
    server.use(
      http.get("/api/approvals", ({ request }) => {
        const state = new URL(request.url).searchParams.get("state");
        if (state === "decided") {
          return HttpResponse.json(
            page([
              decisionFixture({
                approval: {
                  id: "ap-1",
                  ticket_id: "t-3",
                  step_index: 0,
                  name: "Manager review",
                  approver_role: "approver",
                  status: "rejected",
                  comment: "Duplicate of TCK-0002",
                  decided_at: "2026-02-02T09:00:00Z",
                  created_at: "2026-02-01T10:00:00Z",
                },
                ticket: ticketFixture({ id: "t-3", number: 3, title: "Conference budget" }),
              }),
            ]),
          );
        }
        return HttpResponse.json(page([]));
      }),
    );
    await openApprovals();

    await user.click(screen.getByRole("tab", { name: "My decisions" }));
    expect(await screen.findByText("Conference budget")).toBeInTheDocument();
    expect(screen.getByText("rejected")).toBeInTheDocument();
    expect(screen.getByText("Duplicate of TCK-0002")).toBeInTheDocument();
  });
});
