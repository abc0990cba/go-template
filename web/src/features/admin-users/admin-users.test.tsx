import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { page } from "@/test/mocks/handlers";
import { server } from "@/test/mocks/server";
import { renderRoute, seedMe } from "@/test/test-utils";
import type { User } from "@/lib/types";

/**
 * The admin users table, rendered through the real (guarded) route tree:
 * listing with aria-labelled row actions, the create/edit dialogs against
 * POST/PUT /api/users, delete confirmation, and the roles dialog over the
 * assignment API.
 */

const user = userEvent.setup();

function userFixture(overrides: Partial<User> = {}): User {
  return {
    id: "u-1",
    email: "ada@example.com",
    name: "Ada Lovelace",
    created_at: "2026-01-10T00:00:00Z",
    updated_at: "2026-01-10T00:00:00Z",
    ...overrides,
  };
}

function seedAdmin() {
  seedMe({
    permissions: [
      "admin_console:access",
      "users:read",
      "users:create",
      "users:update",
      "users:delete",
      "roles:read",
    ],
  });
  server.use(
    http.get("/api/users", () =>
      HttpResponse.json(
        page([
          userFixture(),
          userFixture({ id: "u-2", email: "grace@example.com", name: "Grace Hopper" }),
        ]),
      ),
    ),
  );
}

async function openUsers() {
  renderRoute("/admin/users");
  await screen.findByRole("heading", { name: "Users" }, { timeout: 4000 });
}

describe("admin users table", () => {
  it("lists users with aria-labelled row actions", async () => {
    seedAdmin();
    await openUsers();

    // The email renders twice per row (mobile span + desktop cell).
    expect((await screen.findAllByText("ada@example.com")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("grace@example.com").length).toBeGreaterThan(0);
    expect(screen.getByLabelText("Open ada@example.com")).toBeInTheDocument();
    expect(screen.getByLabelText("Edit ada@example.com")).toBeInTheDocument();
    expect(screen.getByLabelText("Delete ada@example.com")).toBeInTheDocument();
    expect(screen.getByLabelText("Roles of ada@example.com")).toBeInTheDocument();
  });

  it("creates a user through the dialog", async () => {
    seedAdmin();
    let created: Record<string, unknown> | undefined;
    server.use(
      http.post("/api/users", async ({ request }) => {
        created = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(userFixture({ id: "u-3", email: "new@example.com" }));
      }),
    );
    await openUsers();

    await user.click(await screen.findByRole("button", { name: "New user" }));
    // The dialog title collides with the toolbar button's text.
    expect(
      await screen.findByRole("heading", { name: "New user" }, { timeout: 4000 }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Create" }));
    expect(await screen.findByText("Enter a valid email")).toBeInTheDocument();

    await user.type(screen.getByLabelText("Name"), "New Person");
    await user.type(screen.getByLabelText("Email"), "new@example.com");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(created).toMatchObject({ email: "new@example.com" }));
    expect(await screen.findByText("Created new@example.com")).toBeInTheDocument();
  });

  it("edits a user through the dialog", async () => {
    seedAdmin();
    let updated: Record<string, unknown> | undefined;
    server.use(
      http.put("/api/users/:id", async ({ request }) => {
        updated = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(userFixture());
      }),
    );
    await openUsers();

    await user.click(await screen.findByLabelText("Edit ada@example.com"));
    expect(await screen.findByText("Edit user")).toBeInTheDocument();
    const name = screen.getByLabelText("Name");
    await user.clear(name);
    await user.type(name, "Ada King");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(updated).toMatchObject({ name: "Ada King" }));
    expect(await screen.findByText("User updated")).toBeInTheDocument();
  });

  it("deletes a user behind the confirmation dialog", async () => {
    seedAdmin();
    let deleted = "";
    server.use(
      http.delete("/api/users/:id", ({ params }) => {
        deleted = String(params.id);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    await openUsers();

    await user.click(await screen.findByLabelText("Delete ada@example.com"));
    expect(await screen.findByText("Delete ada@example.com?")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(deleted).toBe("u-1"));
    expect(await screen.findByText("User deleted")).toBeInTheDocument();
  });

  it("assigns and revokes roles in the roles dialog", async () => {
    seedAdmin();
    const assigned: { role: string; domain: string }[] = [];
    let revoked = "";
    server.use(
      http.get("/api/roles", () =>
        HttpResponse.json(
          page([
            {
              key: "requester",
              name: "Requester",
              description: "",
              is_system: true,
              created_at: "2026-01-01T00:00:00Z",
            },
            {
              key: "approver",
              name: "Approver",
              description: "",
              is_system: true,
              created_at: "2026-01-01T00:00:00Z",
            },
          ]),
        ),
      ),
      http.get("/api/users/:id/roles", () =>
        HttpResponse.json(
          page(
            assigned.map((assignment) => ({
              ptype: "g" as const,
              subject: "u-1",
              // g-rules carry the role key in object (user-roles-dialog renders it).
              object: assignment.role,
              domain: assignment.domain,
            })),
          ),
        ),
      ),
      http.post("/api/users/:id/roles", async ({ request }) => {
        assigned.push((await request.json()) as { role: string; domain: string });
        return new HttpResponse(null, { status: 204 });
      }),
      http.delete("/api/users/:id/roles", async ({ request }) => {
        const body = (await request.json()) as { role: string };
        revoked = body.role;
        assigned.length = 0;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    await openUsers();

    await user.click(await screen.findByLabelText("Roles of ada@example.com"));
    expect(await screen.findByText("Roles — ada@example.com")).toBeInTheDocument();
    expect(screen.getByText("No roles assigned.")).toBeInTheDocument();

    // A native <select>, so selectOptions applies directly.
    await user.selectOptions(screen.getByRole("combobox"), "requester");
    await user.click(screen.getByRole("button", { name: "Assign" }));

    await waitFor(() => expect(assigned).toEqual([{ role: "requester", domain: "" }]));
    expect(await screen.findByText("Role assigned")).toBeInTheDocument();
    // The row renders inside the dialog; the hero card's badge behind it
    // shows the same key, so scope the assertion.
    const dialog = screen.getByRole("dialog");
    expect(await within(dialog).findByText("requester")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Revoke requester" }));
    await waitFor(() => expect(revoked).toBe("requester"));
    expect(await screen.findByText("Role revoked")).toBeInTheDocument();
  });
});
