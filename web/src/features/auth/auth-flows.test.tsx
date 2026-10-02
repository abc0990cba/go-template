import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { meFixture } from "@/test/mocks/handlers";
import { server } from "@/test/mocks/server";
import { renderRoute } from "@/test/test-utils";

/**
 * The public auth-flow pages: registration, password reset, email
 * verification, and invitation acceptance. Each is rendered through the
 * real router because they read search params and navigate on success.
 */

const user = userEvent.setup();

async function open(path: string, landmark: string | RegExp) {
  renderRoute(path);
  await screen.findByRole("button", { name: landmark }, { timeout: 4000 });
}

describe("register page", () => {
  it("rejects an empty form with the zod messages", async () => {
    await open("/register", "Create account");
    await user.click(screen.getByRole("button", { name: "Create account" }));
    expect(await screen.findByText("Enter a name")).toBeInTheDocument();
    expect(screen.getByText("Enter a valid email")).toBeInTheDocument();
    expect(screen.getByText("At least 8 characters")).toBeInTheDocument();
  });

  it("auto-logs-in and lands on the dashboard", async () => {
    server.use(
      http.post("/api/auth/register", () =>
        HttpResponse.json({ user: meFixture(), token: "tok-1" }),
      ),
      http.get("/api/tickets/counts", () => HttpResponse.json({})),
      http.get("/api/tickets", () => HttpResponse.json({ data: [], next_cursor: "" })),
      http.get("/api/approvals", () => HttpResponse.json({ data: [], next_cursor: "" })),
      http.get("/api/notifications", () => HttpResponse.json({ data: [], next_cursor: "" })),
    );
    await open("/register", "Create account");
    await user.type(screen.getByLabelText("Name"), "E2E Walker");
    await user.type(screen.getByLabelText("Email"), "walker@example.com");
    await user.type(screen.getByLabelText("Password"), "walking-password-1");
    await user.click(screen.getByRole("button", { name: "Create account" }));

    // Registration issues the session cookie and seeds the session cache.
    expect(await screen.findByText(/Welcome, Tess/, {}, { timeout: 4000 })).toBeInTheDocument();
  });
});

describe("forgot-password page", () => {
  it("always confirms the request without revealing account existence", async () => {
    await open("/forgot-password", "Send reset link");
    await user.type(screen.getByLabelText("Email"), "nobody@example.com");
    await user.click(screen.getByRole("button", { name: "Send reset link" }));
    expect(
      await screen.findByText("If that account exists, a reset link is on its way."),
    ).toBeInTheDocument();
  });
});

describe("reset-password page", () => {
  it("refuses to render the form without a token", async () => {
    await open("/reset-password", "Update password");
    expect(
      screen.getByText("This page needs a token — follow the link from the email."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Update password" })).toBeDisabled();
  });

  it("updates the password and returns to the login card", async () => {
    let body: Record<string, unknown> | undefined;
    server.use(
      http.post("/api/auth/reset-password", async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    await open("/reset-password?token=tok-1", "Update password");
    await user.type(screen.getByLabelText("New password"), "renewed-password-1");
    await user.click(screen.getByRole("button", { name: "Update password" }));

    await waitFor(() => expect(body).toEqual({ token: "tok-1", password: "renewed-password-1" }));
    expect(
      await screen.findByText("Password updated — sign in with the new one."),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole("button", { name: "Sign in" }, { timeout: 4000 }),
    ).toBeInTheDocument();
  });
});

describe("verify-email page", () => {
  it("asks for a token when the link carried none", async () => {
    renderRoute("/verify-email");
    expect(
      await screen.findByText("This page needs a token — follow the link from the email."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Back to login" })).toBeInTheDocument();
  });

  it("verifies the token automatically on mount", async () => {
    let body: Record<string, unknown> | undefined;
    server.use(
      http.post("/api/auth/verify-email", async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderRoute("/verify-email?token=tok-1");

    await waitFor(() => expect(body).toEqual({ token: "tok-1" }), { timeout: 4000 });
    expect(
      await screen.findByText(
        "Your address is confirmed. Everything is set.",
        {},
        {
          timeout: 4000,
        },
      ),
    ).toBeInTheDocument();
  });

  it("stays unresolved and toasts when the token is rejected", async () => {
    server.use(
      http.post("/api/auth/verify-email", () =>
        HttpResponse.json({ title: "invalid token", detail: "expired" }, { status: 400 }),
      ),
    );
    renderRoute("/verify-email?token=tok-1");
    expect(await screen.findByText("expired")).toBeInTheDocument();
    expect(
      screen.queryByText("Your address is confirmed. Everything is set."),
    ).not.toBeInTheDocument();
  });
});

describe("accept-invitation page", () => {
  it("disables acceptance without a token", async () => {
    renderRoute("/accept-invitation");
    expect(
      await screen.findByText(
        "This page needs an invitation token — use the link from your email.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Accept invitation" })).toBeDisabled();
  });

  it("accepts and lands in the invited organization", async () => {
    let body: Record<string, unknown> | undefined;
    server.use(
      http.post("/api/invitations/accept", async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ org_id: "org-acme", role: "member" });
      }),
      http.get("/api/tickets/counts", () => HttpResponse.json({})),
      http.get("/api/tickets", () => HttpResponse.json({ data: [], next_cursor: "" })),
      http.get("/api/approvals", () => HttpResponse.json({ data: [], next_cursor: "" })),
      http.get("/api/notifications", () => HttpResponse.json({ data: [], next_cursor: "" })),
    );
    // The page demands a session first, exactly like the email link flow.
    server.use(http.get("/api/me", () => HttpResponse.json(meFixture())));
    renderRoute("/accept-invitation?token=tok-1");
    await screen.findByRole("button", { name: "Accept invitation" }, { timeout: 4000 });
    await user.click(screen.getByRole("button", { name: "Accept invitation" }));

    await waitFor(() => expect(body).toEqual({ token: "tok-1" }));
    expect(await screen.findByText(/Welcome, Tess/, {}, { timeout: 4000 })).toBeInTheDocument();
  });

  it("explains itself when the invitation is gone", async () => {
    server.use(
      http.get("/api/me", () => HttpResponse.json(meFixture())),
      http.post("/api/invitations/accept", () =>
        HttpResponse.json({ title: "gone", detail: "revoked" }, { status: 410 }),
      ),
    );
    renderRoute("/accept-invitation?token=tok-1");
    await screen.findByRole("button", { name: "Accept invitation" }, { timeout: 4000 });
    await user.click(screen.getByRole("button", { name: "Accept invitation" }));
    expect(
      await screen.findByText(
        "The invitation could not be accepted — it may be revoked, expired, or already used.",
      ),
    ).toBeInTheDocument();
  });
});
