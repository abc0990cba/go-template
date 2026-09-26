import { describe, expect, it } from "vitest";
import { listSearchSchema } from "./search";

describe("listSearchSchema", () => {
  it("defaults to offset mode, page 1", () => {
    const parsed = listSearchSchema.parse({});
    expect(parsed.mode).toBe("offset");
    expect(parsed.page).toBe(1);
  });

  it("coerces page from the query string and clamps garbage via catch", () => {
    expect(listSearchSchema.parse({ page: "3" }).page).toBe(3);
    expect(listSearchSchema.parse({ page: "not-a-number" }).page).toBe(1);
  });

  it("rejects unknown modes into the offset fallback", () => {
    expect(listSearchSchema.parse({ mode: "quantum" }).mode).toBe("offset");
  });
});
