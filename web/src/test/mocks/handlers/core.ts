import { http, HttpResponse } from "msw";
import { page, problem } from "../fixtures";

export const core = [
  http.get("/api/me", () => problem(401, "unauthorized", "no active session")),
  http.get("/api/settings/public", () => HttpResponse.json({ demo_mode: "false" })),
  http.get("/api/notifications", () => HttpResponse.json(page([]))),
  http.get("/api/auth/mfa/status", () =>
    HttpResponse.json({ enabled: false, pending: false, required: false }),
  ),
  http.get("/api/api-keys", () => HttpResponse.json(page([]))),
  http.get("/api/oauth/providers", () => HttpResponse.json(page([]))),
  http.get("/api/me/identities", () => HttpResponse.json(page([]))),
  http.get("/api/sessions", () => HttpResponse.json(page([]))),
  http.get("/api/analytics/summary", () =>
    HttpResponse.json({
      total_users: 0,
      password_identities: 0,
      google_identities: 0,
      github_identities: 0,
      facebook_identities: 0,
      twitter_identities: 0,
      enrichment_succeeded: 0,
      enrichment_failed: 0,
      jobs_queued: 0,
      jobs_done: 0,
      jobs_failed: 0,
    }),
  ),
  http.get("/api/analytics/timeseries", () => HttpResponse.json({ days: 30, data: [] })),
  http.get("/api/notifications/unread-count", () => HttpResponse.json({ count: 0 })),
];
