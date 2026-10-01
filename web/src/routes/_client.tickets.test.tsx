import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { MockEventSource } from "@/test/event-source";
import { page, problem, ticketFixture } from "@/test/mocks/handlers";
import { server } from "@/test/mocks/server";
import { renderRoute, seedMe } from "@/test/test-utils";

/**
 * Integration tests of the tickets page through the real route tree and the
 * MSW-mocked API: list rendering, offset paging, error surfacing, and the
 * SSE push path (LiveProvider) that toasts and refreshes the bell.
 */

const user = userEvent.setup();

function tenTickets(): ReturnType<typeof ticketFixture>[] {
  return Array.from({ length: 10 }, (_, index) =>
    ticketFixture({ id: `t-${index + 1}`, number: index + 1, title: `Ticket ${index + 1}` }),
  );
}

describe("tickets page", () => {
  it("renders the mocked page of tickets with their references", async () => {
    seedMe();
    server.use(
      http.get("/api/tickets", () =>
        HttpResponse.json(page([ticketFixture({ title: "Fix the coffee machine" })])),
      ),
    );

    renderRoute("/tickets");

    expect(
      await screen.findByRole("heading", { name: "My Tickets" }, { timeout: 4000 }),
    ).toBeInTheDocument();
    expect(
      await screen.findByText("Fix the coffee machine", {}, { timeout: 4000 }),
    ).toBeInTheDocument();
    expect(screen.getByText("TCK-0001")).toBeInTheDocument();
  });

  it("pages to the next offset page through the URL-backed controls", async () => {
    seedMe();
    const offsets: number[] = [];
    server.use(
      http.get("/api/tickets", ({ request }) => {
        const search = new URL(request.url).searchParams;
        const offset = Number(search.get("offset") ?? 0);
        offsets.push(offset);
        return offset === 0
          ? HttpResponse.json(page(tenTickets(), "more"))
          : HttpResponse.json(
              page([ticketFixture({ id: "t-11", number: 11, title: "Ticket 11" })]),
            );
      }),
    );

    renderRoute("/tickets");
    await screen.findByText("Ticket 1", {}, { timeout: 4000 });

    await user.click(screen.getByRole("button", { name: "Next" }));
    expect(await screen.findByText("Ticket 11", {}, { timeout: 4000 })).toBeInTheDocument();
    expect(screen.queryByText("Ticket 1")).not.toBeInTheDocument();
    expect(offsets).toEqual([0, 10]);
  });

  it("surfaces a failed creation as a problem+json toast", async () => {
    seedMe();
    server.use(
      http.get("/api/ticket-types", () =>
        HttpResponse.json(
          page([
            {
              id: "tt-generic",
              key: "generic",
              name: "Generic request",
              description: "",
              steps: [{ name: "Manager review", approver_role: "approver" }],
              active: true,
              created_at: "2026-01-01T00:00:00Z",
              updated_at: "2026-01-01T00:00:00Z",
            },
          ]),
        ),
      ),
      // Mutation errors reach the user as toasts carrying the problem detail.
      http.post("/api/tickets", () => problem(409, "conflict", "the type is retired")),
    );

    renderRoute("/tickets");
    await screen.findByRole("heading", { name: "My Tickets" }, { timeout: 4000 });

    await user.click(screen.getByRole("button", { name: "New ticket" }));
    await user.click(screen.getByLabelText("Ticket type"));
    await user.click(await screen.findByText("Generic request — 1 step", {}, { timeout: 4000 }));
    await user.type(screen.getByLabelText("Title"), "New desk chair");
    await user.click(screen.getByRole("button", { name: "Create draft" }));

    expect(
      await screen.findByText("the type is retired", {}, { timeout: 4000 }),
    ).toBeInTheDocument();
  });

  it("pushes live notifications as a toast and refreshes the unread count", async () => {
    const me = seedMe();
    let unreadFetches = 0;
    server.use(
      http.get("/api/notifications/unread-count", () => {
        unreadFetches += 1;
        return HttpResponse.json({ count: 1 });
      }),
    );

    renderRoute("/tickets");
    await screen.findByRole("heading", { name: "My Tickets" }, { timeout: 4000 });

    // LiveProvider opened the single SSE connection of the app on mount.
    const stream = MockEventSource.last;
    expect(stream.url).toBe("/api/events/stream");
    stream.open();
    stream.message({
      type: "notification.created",
      payload: {
        id: "n-1",
        user_id: me.id,
        type: "ticket.approved",
        title: "Ticket approved",
        body: "TCK-0001 cleared its first step",
        link: "/tickets/t-1",
        read_at: null,
        created_at: "2026-02-02T10:00:00Z",
      },
    });

    expect(await screen.findByText("Ticket approved", {}, { timeout: 4000 })).toBeInTheDocument();
    await waitFor(() => expect(unreadFetches).toBeGreaterThan(1), { timeout: 4000 });
  });

  it("shows the org-wide tab only to holders of tickets:read_all", async () => {
    seedMe({ permissions: ["tickets:read", "tickets:create", "tickets:read_all"] });
    server.use(http.get("/api/tickets", () => HttpResponse.json(page(tenTickets()))));

    renderRoute("/tickets");

    expect(
      await screen.findByRole("tab", { name: "All (org)" }, { timeout: 4000 }),
    ).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "My tickets" })).toBeInTheDocument();
  });

  it("opens the create dialog and files a ticket against the API", async () => {
    seedMe();
    let created: Record<string, unknown> | undefined;
    server.use(
      http.get("/api/ticket-types", () =>
        HttpResponse.json(
          page([
            {
              id: "tt-generic",
              key: "generic",
              name: "Generic request",
              description: "",
              steps: [{ name: "Manager review", approver_role: "approver" }],
              active: true,
              created_at: "2026-01-01T00:00:00Z",
              updated_at: "2026-01-01T00:00:00Z",
            },
          ]),
        ),
      ),
      http.post("/api/tickets", async ({ request }) => {
        created = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(
          ticketFixture({ id: "t-new", number: 12, title: "New desk chair", status: "draft" }),
        );
      }),
    );

    renderRoute("/tickets");
    await screen.findByRole("heading", { name: "My Tickets" }, { timeout: 4000 });

    await user.click(screen.getByRole("button", { name: "New ticket" }));
    // Radix Select needs two interactions: open, then pick the visible item.
    await user.click(screen.getByLabelText("Ticket type"));
    await user.click(await screen.findByText("Generic request — 1 step", {}, { timeout: 4000 }));
    await user.type(screen.getByLabelText("Title"), "New desk chair");
    await user.click(screen.getByRole("button", { name: "Create draft" }));

    await waitFor(() =>
      expect(created).toMatchObject({ type_id: "tt-generic", title: "New desk chair" }),
    );
    expect(
      await screen.findByText(/TCK-0012 created as a draft/, {}, { timeout: 4000 }),
    ).toBeInTheDocument();
  });
});
