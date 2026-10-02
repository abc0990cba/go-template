import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";
import { meFixture, page, problem } from "@/test/mocks/handlers";
import { server } from "@/test/mocks/server";
import { renderRoute, seedMe } from "@/test/test-utils";

// QRCode.toDataURL rasterizes through canvas, which jsdom does not
// implement; the component already falls back to the raw secret when it
// rejects, but the mock keeps the <img> render path under test too.
vi.mock("qrcode", () => ({
  default: { toDataURL: vi.fn().mockResolvedValue("data:image/png;base64,FAKEQR") },
}));

/**
 * Integration tests of the profile page: the OAuth identity section
 * (provider states, unlink), session listing, the TOTP lifecycle
 * (enroll → confirm → recovery codes, disable), API keys, and the
 * oauth_error banner the OAuth callback redirects back with.
 */

const user = userEvent.setup();

const identity = (overrides: Partial<Record<string, unknown>> = {}) => ({
  user_id: "u-test-1",
  provider: "github",
  provider_subject: "gh-123",
  email: "tess@github.example",
  name: "Tess on GitHub",
  avatar_url: "",
  linked_at: "2026-01-05T00:00:00Z",
  last_login_at: "2026-02-01T00:00:00Z",
  ...overrides,
});

const session = (overrides: Partial<Record<string, unknown>> = {}) => ({
  id: "sess-abcd1234",
  created_at: "2026-02-01T08:00:00Z",
  expires_at: "2026-02-08T08:00:00Z",
  current: true,
  ...overrides,
});

const provider = (name: string, configured: boolean) => ({ name, configured });

/** Seeds a session plus the identity/session/provider fixtures of a
 * password-only account with every OAuth provider configured. */
function seedProfilePage() {
  seedMe();
  server.use(
    http.get("/api/me/identities", () =>
      HttpResponse.json(page([identity({ provider: "password", email: "tess@example.com" })])),
    ),
    http.get("/api/sessions", () =>
      HttpResponse.json(page([session(), session({ id: "sess-ffff0000", current: false })])),
    ),
    http.get("/api/oauth/providers", () =>
      HttpResponse.json(
        page([
          provider("google", true),
          provider("github", true),
          provider("facebook", false),
          provider("twitter", false),
        ]),
      ),
    ),
  );
}

async function openProfile(path = "/profile") {
  renderRoute(path);
  await screen.findByRole("heading", { name: "Profile" }, { timeout: 4000 });
}

describe("profile page", () => {
  it("renders the oauth_error banner the callback redirects back with", async () => {
    seedProfilePage();
    await openProfile(`/profile?oauth_error=${encodeURIComponent("Provider said no")}`);
    expect(await screen.findByText("Provider said no")).toBeInTheDocument();
  });

  it("reflects provider configuration and linked state on the link buttons", async () => {
    seedProfilePage();
    await openProfile();

    // Configured but not linked: enabled, labelled "Link …".
    const github = await screen.findByRole("button", { name: "Link github" });
    expect(github).toBeEnabled();
    expect(screen.getByRole("button", { name: "Link google" })).toBeEnabled();
    // Unconfigured: disabled, the tooltip says why.
    const facebook = screen.getByRole("button", { name: "Link facebook" });
    expect(facebook).toBeDisabled();
    expect(facebook).toHaveAttribute(
      "title",
      "facebook client credentials are not configured on the backend",
    );
    // Password is a credential row, not an unlinkable identity.
    expect(screen.getByText("credential")).toBeInTheDocument();
  });

  it("unlinks a connected identity and refreshes the session probe", async () => {
    seedProfilePage();
    let unlinked = "";
    let meFetches = 0;
    server.use(
      http.get("/api/me/identities", () =>
        HttpResponse.json(
          page([
            identity({ provider: "password", email: "tess@example.com" }),
            identity({ provider: "github" }),
          ]),
        ),
      ),
      http.post("/api/profile/unlink/:provider", ({ params }) => {
        unlinked = String(params.provider);
        return new HttpResponse(null, { status: 204 });
      }),
      http.get("/api/me", () => {
        meFetches += 1;
        return HttpResponse.json(meFixture());
      }),
    );
    await openProfile();

    // One "github linked" button (linked) and one row with an Unlink button.
    expect(await screen.findByText("github linked")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Unlink/ }));

    await waitFor(() => expect(unlinked).toBe("github"));
    expect(await screen.findByText("Identity unlinked")).toBeInTheDocument();
    expect(meFetches).toBeGreaterThan(0);
  });

  it("marks the current session and lists other devices", async () => {
    seedProfilePage();
    await openProfile();
    expect(await screen.findByText("this session")).toBeInTheDocument();
    expect(screen.getByText("other device")).toBeInTheDocument();
  });

  it("walks the TOTP lifecycle: enroll, confirm, recovery codes", async () => {
    seedProfilePage();
    let verifyBody: Record<string, unknown> | undefined;
    server.use(
      http.post("/api/auth/mfa/enroll", () =>
        HttpResponse.json({ secret: "JBSWY3DPEHPK3PXP", otpauth_url: "otpauth://totp/x" }),
      ),
      http.post("/api/auth/mfa/verify", async ({ request }) => {
        verifyBody = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ recovery_codes: ["1111-2222", "3333-4444"] });
      }),
    );
    await openProfile();

    await user.click(await screen.findByRole("button", { name: "Set up two-factor" }));
    expect(
      await screen.findByAltText("TOTP provisioning QR code", {}, { timeout: 4000 }),
    ).toBeInTheDocument();
    expect(screen.getByText("JBSWY3DPEHPK3PXP")).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText("6-digit code"), "123456");
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    expect(await screen.findByText(/Save your recovery codes now/)).toBeInTheDocument();
    expect(screen.getByText("1111-2222")).toBeInTheDocument();
    expect(verifyBody).toEqual({ code: "123456" });
    await user.click(screen.getByRole("button", { name: "I saved them" }));
    expect(screen.queryByText(/Save your recovery codes now/)).not.toBeInTheDocument();
  });

  it("disables an enabled factor with password and current code", async () => {
    seedMe();
    server.use(
      http.get("/api/me/identities", () => HttpResponse.json(page([]))),
      http.get("/api/sessions", () => HttpResponse.json(page([]))),
      http.get("/api/auth/mfa/status", () =>
        HttpResponse.json({ enabled: true, pending: false, required: false }),
      ),
      http.post("/api/auth/mfa/disable", () => new HttpResponse(null, { status: 204 })),
    );
    await openProfile();

    expect(await screen.findByText(/Enabled — sign-in asks for a code/)).toBeInTheDocument();
    const disable = screen.getByRole("button", { name: "Disable" });
    expect(disable).toBeDisabled();

    await user.type(screen.getByPlaceholderText("Password"), "demo-password-123");
    await user.type(screen.getByPlaceholderText("Current code"), "123456");
    await user.click(disable);
    expect(await screen.findByText("Two-factor authentication disabled")).toBeInTheDocument();
  });

  it("shows an API key secret exactly once and revokes keys", async () => {
    seedProfilePage();
    const listedKey = {
      id: "key-1",
      user_id: "u-test-1",
      name: "ci",
      prefix: "gta_1",
      scopes: ["users:read"],
      last_used_at: null,
      expires_at: null,
      revoked_at: null,
      created_at: "2026-02-02T00:00:00Z",
    };
    server.use(
      // The list refetches after creation, which is when the revoke button
      // (rendered per listed key) becomes available.
      http.get("/api/api-keys", () => HttpResponse.json(page([listedKey]))),
      http.post("/api/api-keys", () =>
        HttpResponse.json({ key: listedKey, secret: "gta_1_secret_value" }),
      ),
      http.delete("/api/api-keys/:id", () => new HttpResponse(null, { status: 204 })),
    );
    await openProfile();

    await user.type(screen.getByPlaceholderText("Key name"), "ci");
    await user.type(screen.getByPlaceholderText("scopes: users:read, jobs:*"), "users:read");
    await user.click(screen.getByRole("button", { name: "Create key" }));

    expect(await screen.findByText(/Copy your key now/)).toBeInTheDocument();
    expect(screen.getByText("gta_1_secret_value")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Revoke ci" }));
    expect(await screen.findByText("Key revoked")).toBeInTheDocument();
  });

  it("logs out everywhere and returns to the login card", async () => {
    seedProfilePage();
    server.use(http.post("/api/profile/logout-all", () => new HttpResponse(null, { status: 204 })));
    const { router } = renderRoute("/profile");
    await screen.findByRole("heading", { name: "Profile" }, { timeout: 4000 });

    await user.click(screen.getByRole("button", { name: "Log out everywhere" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/login"), {
      timeout: 4000,
    });
    expect(
      await screen.findByRole("button", { name: "Sign in" }, { timeout: 4000 }),
    ).toBeInTheDocument();
  });

  it("surfaces unlink failures as a problem toast", async () => {
    seedProfilePage();
    server.use(
      http.get("/api/me/identities", () =>
        HttpResponse.json(page([identity({ provider: "github" })])),
      ),
      http.post("/api/profile/unlink/:provider", () =>
        problem(409, "conflict", "unlink the last credential first"),
      ),
    );
    await openProfile();

    await user.click(await screen.findByRole("button", { name: /Unlink/ }));
    expect(await screen.findByText("unlink the last credential first")).toBeInTheDocument();
  });
});
