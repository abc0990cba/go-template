import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { ApiError, api, unwrap, setActiveOrg } from "./core";
import createClient from "openapi-fetch";
import type { paths } from "@/shared/api/schema.gen";
import { meFixture, problem } from "@/test/mocks/handlers";
import { server } from "@/test/mocks/server";

// The transport pipeline is exercised through /api/me and /api/auth/logout
// so the tests keep asserting end-user-visible behavior of the core module.
const me = () => unwrap(api.GET("/api/me"));
const logout = () => unwrap(api.POST("/api/auth/logout"));

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
    const error = await me().then(
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
    await expect(me()).rejects.toMatchObject({ status: 504, title: "request failed" });
  });

  it("maps network failures to ApiError with status 0", async () => {
    server.use(http.get("/api/me", () => HttpResponse.error()));
    await expect(me()).rejects.toMatchObject({ status: 0 });
  });

  it("returns undefined for 204 responses", async () => {
    await expect(logout()).resolves.toBeUndefined();
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
    await me();
    setActiveOrg("");
    await me();
    expect(seenOrgHeaders).toEqual(["org-acme", null]);
  });
});

describe("transportFetch preserves the typed client's Request headers", () => {
  // openapi-fetch builds a Request (body + Content-Type already set) and
  // calls the injected fetch as fetch(request, undefined). The wrapper must
  // not pass an empty `headers` init member: per the fetch spec that member
  // REPLACES the Request's header list, and the body went out with an empty
  // Content-Type — the runtime contract gate rejected every browser write.
  it("keeps Content-Type when openapi-fetch passes a Request as input", async () => {
    let seen: { contentType: string; hasBody: boolean } | null = null;
    const spyFetch: typeof fetch = async (input, init) => {
      // Mirror what a real fetch does: init members override the Request.
      const request = new Request(input, init);
      seen = {
        contentType: request.headers.get("Content-Type") ?? "",
        hasBody: request.body !== null,
      };
      return new Response(JSON.stringify({ ok: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    };

    const client = createClient<paths>({ baseUrl: "http://mock.test", fetch: spyFetch });
    await client.POST("/api/auth/login", { body: { email: "a@b.c", password: "x" } });

    expect(seen).not.toBeNull();
    // @ts-expect-error narrowing for the assertion
    expect(seen.contentType).toBe("application/json");
    // @ts-expect-error narrowing for the assertion
    expect(seen.hasBody).toBe(true);
  });

  it("sends Content-Type AND the tenant header together through the real wrapper", async () => {
    setActiveOrg("org-e2e");
    // The wrapper is the global api's fetch; MSW sees the headers exactly
    // as the server would. Before the header-preservation fix, this request
    // went out with Content-Type "" (wiped) and the contract gate 400'd it.
    let atServer: { contentType: string; orgId: string; bodyOk: boolean } | null = null;
    server.use(
      http.post("/api/auth/login", async ({ request }) => {
        atServer = {
          contentType: request.headers.get("Content-Type") ?? "",
          orgId: request.headers.get("X-Org-Id") ?? "",
          bodyOk: (await request.json()) != null,
        };
        return HttpResponse.json({ user: {}, token: "t" });
      }),
    );
    await api.POST("/api/auth/login", { body: { email: "a@b.c", password: "x" } });

    expect(atServer).not.toBeNull();
    // @ts-expect-error narrowing for the assertion
    expect(atServer.contentType).toBe("application/json");
    // @ts-expect-error narrowing for the assertion
    expect(atServer.orgId).toBe("org-e2e");
    // @ts-expect-error narrowing for the assertion
    expect(atServer.bodyOk).toBe(true);
    setActiveOrg("");
  });
});
