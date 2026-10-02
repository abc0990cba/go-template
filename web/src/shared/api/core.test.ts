import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { ApiError, http as transport, setActiveOrg } from "./core";
import { meFixture, problem } from "@/test/mocks/handlers";
import { server } from "@/test/mocks/server";

// The transport pipeline is exercised through thin api.* wrappers so the
// tests keep asserting end-user-visible behavior of the core module.
const api = {
  me: () => transport.get("/api/me"),
  logout: () => transport.post("/api/auth/logout"),
};
describe("ApiError", () => {
  it("prefers the detail for the message and keeps the title", () => {
    const error = new ApiError(409, "conflict", "duplicate email");
    expect(error.status).toBe(409);
    expect(error.title).toBe("conflict");
    expect(error.message).toBe("duplicate email");
  });
});

describe("request pipeline", () => {
  afterEach(() => setActiveOrg(""));

  it("parses problem+json errors into ApiError", async () => {
    server.use(http.get("/api/me", () => problem(403, "forbidden", "not your org")));
    const error = await api.me().then(
      () => {
        throw new Error("expected the request to fail");
      },
      (caught: unknown) => caught,
    );
    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ status: 403, title: "forbidden", message: "not your org" });
  });

  it("keeps non-JSON error bodies meaningful via the status", async () => {
    server.use(http.get("/api/me", () => new HttpResponse("gateway timeout", { status: 504 })));
    await expect(api.me()).rejects.toMatchObject({ status: 504, title: "request failed" });
  });

  it("maps network failures to ApiError with status 0", async () => {
    server.use(http.get("/api/me", () => HttpResponse.error()));
    await expect(api.me()).rejects.toMatchObject({ status: 0 });
  });

  it("returns undefined for 204 responses", async () => {
    await expect(api.logout()).resolves.toBeUndefined();
  });

  it("sends the active org as X-Org-Id and drops the header when empty", async () => {
    const seenOrgHeaders: (string | null)[] = [];
    server.use(
      http.get("/api/me", ({ request }) => {
        seenOrgHeaders.push(request.headers.get("X-Org-Id"));
        return HttpResponse.json(meFixture());
      }),
    );
    setActiveOrg("org-acme");
    await api.me();
    setActiveOrg("");
    await api.me();
    expect(seenOrgHeaders).toEqual(["org-acme", null]);
  });
});
