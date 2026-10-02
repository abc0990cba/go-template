import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { meFixture } from "@/test/mocks/handlers";
import { server } from "@/test/mocks/server";
import { renderRoute } from "@/test/test-utils";

/**
 * Component-layer tests of the login card: validation, the two-step MFA
 * protocol, redirect-back, and demo mode. The route is rendered through the
 * real router (renderRoute) because the card reads search params and
 * navigates after success.
 */

const user = userEvent.setup();

/** The router mounts asynchronously; every interaction waits for the card. */
async function openLoginForm() {
  await screen.findByRole("button", { name: "Sign in" });
}

async function submitLoginForm() {
  await user.click(screen.getByRole("button", { name: "Sign in" }));
}

describe("login page", () => {
  it("rejects an empty form with the zod messages", async () => {
    renderRoute("/login");
    await openLoginForm();
    await user.clear(screen.getByPlaceholderText("you@example.com"));
    await user.clear(screen.getByPlaceholderText("••••••••"));
    await submitLoginForm();
    expect(await screen.findByText("Enter an email")).toBeInTheDocument();
    expect(screen.getByText("At least 8 characters")).toBeInTheDocument();
  });

  it("surfaces the problem detail when the password step fails", async () => {
    renderRoute("/login");
    await openLoginForm();
    // Default handler: 401 problem with detail "email or password is wrong".
    await submitLoginForm();
    expect(await screen.findByText("email or password is wrong")).toBeInTheDocument();
  });

  it("switches to the TOTP step when the password answers mfa_required", async () => {
    server.use(
      http.post("/api/auth/login", () =>
        HttpResponse.json({ mfa_required: true, challenge: "ch-123" }, { status: 401 }),
      ),
      http.post("/api/auth/mfa/challenge", () =>
        HttpResponse.json({ user: meFixture(), token: "tok-1" }),
      ),
      http.get("/api/me", () => HttpResponse.json(meFixture())),
    );
    renderRoute("/login");
    await openLoginForm();

    await submitLoginForm();
    expect(await screen.findByText("Authentication code")).toBeInTheDocument();

    await user.type(screen.getByLabelText("Authentication code"), "123456");
    await user.click(screen.getByRole("button", { name: "Verify and sign in" }));
    // finishLogin resolves the fresh session probe, then lands on /dashboard —
    // several async hops, so allow more than findBy's 1s default.
    expect(await screen.findByText(/Welcome, Tess/, {}, { timeout: 4000 })).toBeInTheDocument();
  });

  it("returns to the saved location after signing in", async () => {
    server.use(
      http.post("/api/auth/login", () => HttpResponse.json({ user: meFixture(), token: "tok-1" })),
      http.get("/api/me", () => HttpResponse.json(meFixture())),
    );
    renderRoute("/login?redirect=/tickets");
    await openLoginForm();

    await submitLoginForm();
    expect(
      await screen.findByRole("heading", { name: "My Tickets" }, { timeout: 4000 }),
    ).toBeInTheDocument();
  });

  it("restores the full table state of a redirect target with search params", async () => {
    server.use(
      http.post("/api/auth/login", () => HttpResponse.json({ user: meFixture(), token: "tok-1" })),
      http.get("/api/me", () => HttpResponse.json(meFixture())),
    );
    // The guard encodes location.href, query string included; the ticket
    // list keeps its table state in the URL and must get it back.
    const redirect = encodeURIComponent("/tickets?mode=cursor&view=paged&page=2");
    const { router } = renderRoute(`/login?redirect=${redirect}`);
    await openLoginForm();

    await submitLoginForm();
    await waitFor(() => expect(router.state.location.pathname).toBe("/tickets"), {
      timeout: 4000,
    });
    expect(router.state.location.search).toMatchObject({
      mode: "cursor",
      view: "paged",
      page: 2,
    });
  });

  it.each(["https://evil.com", "//evil.com"])(
    "falls back to the dashboard for the unsafe redirect %s",
    async (unsafe) => {
      server.use(
        http.post("/api/auth/login", () =>
          HttpResponse.json({ user: meFixture(), token: "tok-1" }),
        ),
        http.get("/api/me", () => HttpResponse.json(meFixture())),
      );
      const { router } = renderRoute(`/login?redirect=${encodeURIComponent(unsafe)}`);
      await openLoginForm();

      await submitLoginForm();
      await waitFor(() => expect(router.state.location.pathname).toBe("/dashboard"), {
        timeout: 4000,
      });
    },
  );

  it("renders the demo picker when demo mode is on", async () => {
    server.use(http.get("/api/settings/public", () => HttpResponse.json({ demo_mode: "true" })));
    renderRoute("/login");

    expect(await screen.findByRole("heading", { name: "Demo accounts" })).toBeInTheDocument();
    expect(screen.getByText("Rita Requester")).toBeInTheDocument();
    // The pre-filled demo credentials alert only shows outside demo mode.
    await waitFor(() => expect(screen.queryByText("Demo credentials")).not.toBeInTheDocument());
  });
});
