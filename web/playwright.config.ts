import { defineConfig, devices } from "@playwright/test";

/**
 * Browser E2E over the real thing: the Go binary serving the embedded SPA
 * on the in-memory store (bootstrap falls back to MemoryStore when
 * APP_DATABASE_URL is empty), seeded with the demo dataset so journeys
 * have ticket types, tickets, and roles to walk through.
 *
 * Set E2E_BASE_URL to point the specs at an already-running stack and skip
 * the managed webServer entirely — the same escape hatch the Go e2e suite
 * uses with GO_TEMPLATE_BASE_URL.
 */

const PORT = Number(process.env.E2E_PORT ?? 8090);
const baseURL = process.env.E2E_BASE_URL ?? `http://127.0.0.1:${PORT}`;

// Superadmin seeded on boot (bootstrap.SeedDemoUser) for admin journeys.
process.env.E2E_ADMIN_EMAIL ??= "e2e-admin@example.com";
process.env.E2E_ADMIN_PASSWORD ??= "e2e-admin-password-123";

export default defineConfig({
  testDir: "./e2e",
  // One worker: the specs share one seeded server, and journeys create
  // records in it. Cookie jars stay isolated per test via fresh contexts.
  workers: 1,
  fullyParallel: false,
  timeout: 30_000,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : {
        command: [
          "npm run build",
          "cd .. && go build -o web/e2e/.tmp/api-server ./cmd/api",
          // exec keeps this pid as the direct child (Playwright's teardown
          // signal lands on the server), and env(1) actually exports the
          // assignments — `VAR=x exec cmd` would not.
          `exec env APP_DATABASE_URL= APP_DEMO_SEED=1 APP_HTTP_ADDR=127.0.0.1:${PORT} APP_DEMO_EMAIL=${process.env.E2E_ADMIN_EMAIL} APP_DEMO_PASSWORD=${process.env.E2E_ADMIN_PASSWORD} ./web/e2e/.tmp/api-server`,
        ].join(" && "),
        url: `http://127.0.0.1:${PORT}/health/ready`,
        reuseExistingServer: !process.env.CI,
        timeout: 300_000,
        stdout: "pipe",
        stderr: "pipe",
      },
});
