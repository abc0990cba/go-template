import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { ApiError, api, buildListQuery, setActiveOrg } from "./api";
import { meFixture, problem } from "@/test/mocks/handlers";
import { server } from "@/test/mocks/server";

describe("buildListQuery", () => {
  it("always carries mode and limit", () => {
    const query = buildListQuery({ mode: "offset", limit: 10 });
    expect(query).toContain("mode=offset");
    expect(query).toContain("limit=10");
  });

  it("sends offset only in offset mode and after only in cursor modes", () => {
    expect(buildListQuery({ mode: "offset", limit: 10, offset: 30 })).toContain("offset=30");
    expect(buildListQuery({ mode: "offset", limit: 10, offset: 0 })).not.toContain("offset=");
    expect(buildListQuery({ mode: "keyset", limit: 10, after: "abc" })).toContain("after=abc");
    expect(buildListQuery({ mode: "cursor", limit: 10, offset: 30 })).not.toContain("offset=30");
  });

  it("passes optional filters through", () => {
    const query = buildListQuery({
      mode: "offset",
      limit: 5,
      q: "ada",
      status: "done",
      user_id: "u1",
    });
    expect(query).toContain("q=ada");
    expect(query).toContain("status=done");
    expect(query).toContain("user_id=u1");
  });
});

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

describe("login outcome protocol", () => {
  it("classifies the mfa_required answer without throwing", async () => {
    // The backend answers the two intentional non-200s with 401 + a marker body
    // (internal/auth/http.go); loginStep must not surface them as ApiError.
    server.use(
      http.post("/api/auth/login", () =>
        HttpResponse.json({ mfa_required: true, challenge: "ch-123" }, { status: 401 }),
      ),
    );
    await expect(api.loginStep("a@b.c", "password123")).resolves.toEqual({
      kind: "mfa_required",
      challenge: "ch-123",
    });
    // The raw login() helper refuses to bypass the second step.
    await expect(api.login("a@b.c", "password123")).rejects.toMatchObject({
      status: 401,
      title: "mfa_required",
    });
  });

  it("classifies the enrollment gate", async () => {
    server.use(
      http.post("/api/auth/login", () =>
        HttpResponse.json({ mfa_enrollment_required: true }, { status: 401 }),
      ),
    );
    await expect(api.loginStep("a@b.c", "password123")).resolves.toEqual({
      kind: "enrollment_required",
    });
  });

  it("resolves the ok outcome into user and token", async () => {
    const user = meFixture();
    server.use(http.post("/api/auth/login", () => HttpResponse.json({ user, token: "tok-1" })));
    await expect(api.login("a@b.c", "password123")).resolves.toEqual({ user, token: "tok-1" });
  });
});
