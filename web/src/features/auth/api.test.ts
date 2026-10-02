import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { login, loginStep } from "./api";
import { meFixture } from "@/test/mocks/handlers";
import { server } from "@/test/mocks/server";

describe("login outcome protocol", () => {
  it("classifies the mfa_required answer without throwing", async () => {
    // The backend answers the two intentional non-200s with 401 + a marker body
    // (internal/auth/http.go); loginStep must not surface them as ApiError.
    server.use(
      http.post("/api/auth/login", () =>
        HttpResponse.json({ mfa_required: true, challenge: "ch-123" }, { status: 401 }),
      ),
    );
    await expect(loginStep("a@b.c", "password123")).resolves.toEqual({
      kind: "mfa_required",
      challenge: "ch-123",
    });
    // The raw login() helper refuses to bypass the second step.
    await expect(login("a@b.c", "password123")).rejects.toMatchObject({
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
    await expect(loginStep("a@b.c", "password123")).resolves.toEqual({
      kind: "enrollment_required",
    });
  });

  it("resolves the ok outcome into user and token", async () => {
    const user = meFixture();
    server.use(http.post("/api/auth/login", () => HttpResponse.json({ user, token: "tok-1" })));
    await expect(login("a@b.c", "password123")).resolves.toEqual({ user, token: "tok-1" });
  });
});
