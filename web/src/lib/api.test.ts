import { describe, expect, it } from "vitest";
import { ApiError, buildListQuery } from "./api";

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
