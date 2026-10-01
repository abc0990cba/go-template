import { beforeEach, describe, expect, it } from "vitest";
import { ensureAuth, ensurePermission } from "./auth";
import { queryClient } from "./router";
import { meFixture } from "@/test/mocks/handlers";

// These guards run through the shared query client (lib/router); MSW's
// default handlers own the network: GET /api/me answers 401 anonymously.
beforeEach(() => {
  queryClient.clear();
});

describe("ensureAuth", () => {
  it("resolves when the session query has data", async () => {
    queryClient.setQueryData(["me"], meFixture());
    await expect(ensureAuth({ href: "/tickets" })).resolves.toBeUndefined();
  });

  it("redirects anonymous visitors to /login, remembering where they headed", async () => {
    // redirect() throws a Response-like object; its routing payload lives in options.
    await expect(ensureAuth({ href: "/tickets" })).rejects.toMatchObject({
      options: { to: "/login", search: { redirect: "/tickets" } },
    });
  });
});

describe("ensurePermission", () => {
  it("passes when the session carries the object:action permission", async () => {
    queryClient.setQueryData(["me"], meFixture({ permissions: ["admin_console:access"] }));
    await expect(
      ensurePermission("admin_console", "access", { href: "/admin" }),
    ).resolves.toBeUndefined();
  });

  it("redirects to the dashboard when the permission is missing", async () => {
    queryClient.setQueryData(["me"], meFixture({ permissions: ["tickets:read"] }));
    await expect(
      ensurePermission("admin_console", "access", { href: "/admin" }),
    ).rejects.toMatchObject({ options: { to: "/dashboard" } });
  });

  it("demands a session before judging permissions", async () => {
    await expect(
      ensurePermission("admin_console", "access", { href: "/admin" }),
    ).rejects.toMatchObject({ options: { to: "/login" } });
  });
});
