import { screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderRoute, seedMe } from "@/test/test-utils";

/**
 * The /admin guard lives in the route tree (beforeLoad on the admin
 * layout), so it is exercised through the real router: a requester who
 * types the URL is redirected to /dashboard, an account holding
 * admin_console:access gets in.
 */
describe("admin area guard", () => {
  it("redirects accounts without admin_console:access to the dashboard", async () => {
    seedMe({ permissions: ["tickets:read"] });
    const { router } = renderRoute("/admin");

    await waitFor(() => expect(router.state.location.pathname).toBe("/dashboard"), {
      timeout: 4000,
    });
    expect(await screen.findByText(/Welcome, Tess/)).toBeInTheDocument();
  });

  it("admits accounts holding the access permission", async () => {
    seedMe({
      permissions: ["admin_console:access", "users:read", "orgs:read", "tickets:read_all"],
    });
    const { router } = renderRoute("/admin");

    await waitFor(() => expect(router.state.location.pathname).toBe("/admin"), { timeout: 4000 });
    // The admin shell's sidebar groups render once the layout admits us.
    expect(await screen.findByText("Platform", {}, { timeout: 4000 })).toBeInTheDocument();
    expect(screen.getAllByText("Administration").length).toBeGreaterThan(0);
  });
});
