import { http, HttpResponse } from "msw";
import type { Me, Paged, Ticket, TicketDecision, TicketType } from "@/lib/types";

/**
 * Default MSW handlers: the API surface the SPA renders against, as plain
 * functions of the real contracts in internal/httpapp. The baseline is an
 * anonymous visitor — GET /api/me answers 401 — so route guards behave
 * exactly as they do for a signed-out browser. Authenticated scenarios
 * either seed the session cache (seedMe in test-utils) or override a
 * handler per test with server.use().
 */

/** An RFC-7807 problem response, the error shape every endpoint returns. */
export function problem(status: number, title: string, detail = ""): Response {
  return HttpResponse.json({ status, title, detail }, { status });
}

export function page<T>(data: T[], next_cursor = ""): Paged<T> {
  return { data, next_cursor };
}

/** A minimal valid session probe; overrides layer on top of it. */
export function meFixture(overrides: Partial<Me> = {}): Me {
  return {
    id: "u-test-1",
    email: "tess@example.com",
    name: "Tess Tester",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    orgs: [],
    active_org: "",
    roles: ["requester"],
    permissions: ["tickets:read", "tickets:create"],
    impersonated: false,
    ...overrides,
  };
}

export function ticketFixture(overrides: Partial<Ticket> = {}): Ticket {
  return {
    id: "t-1",
    number: 1,
    type_id: "tt-generic",
    type_key: "generic",
    title: "Fix the coffee machine",
    description: "It stopped brewing.",
    priority: "medium",
    status: "draft",
    current_step: 0,
    created_by: "u-test-1",
    decided_at: null,
    created_at: "2026-02-01T10:00:00Z",
    updated_at: "2026-02-01T10:00:00Z",
    ...overrides,
  };
}

export function decisionFixture(overrides: Partial<TicketDecision> = {}): TicketDecision {
  return {
    approval: {
      id: "ap-1",
      ticket_id: "t-1",
      step_index: 0,
      name: "Manager review",
      approver_role: "approver",
      status: "approved",
      decided_at: "2026-02-02T09:00:00Z",
      created_at: "2026-02-01T10:00:00Z",
    },
    ticket: ticketFixture({ status: "in_review" }),
    ...overrides,
  };
}

export function ticketTypeFixture(overrides: Partial<TicketType> = {}): TicketType {
  return {
    id: "tt-generic",
    key: "generic",
    name: "Generic request",
    description: "A single-step approval chain.",
    steps: [{ name: "Manager review", approver_role: "approver" }],
    active: true,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

export const handlers = [
  // Session: anonymous by default; tests that need a session seed the cache
  // (seedMe) so the 60s staleTime keeps guards off the network entirely.
  http.get("/api/me", () => problem(401, "unauthorized", "no active session")),
  http.post("/api/auth/login", () =>
    problem(401, "invalid credentials", "email or password is wrong"),
  ),
  http.post("/api/auth/mfa/challenge", () =>
    problem(401, "invalid code", "the code is wrong or expired"),
  ),
  http.post("/api/auth/logout", () => new HttpResponse(null, { status: 204 })),

  http.get("/api/settings/public", () => HttpResponse.json({ demo_mode: "false" })),

  http.get("/api/notifications", () => HttpResponse.json(page([]))),
  http.get("/api/notifications/unread-count", () => HttpResponse.json({ count: 0 })),

  http.get("/api/tickets", () => HttpResponse.json(page([]))),
  http.get("/api/tickets/counts", () =>
    HttpResponse.json({
      draft: 0,
      in_review: 0,
      approved: 0,
      completed: 0,
      rejected: 0,
      withdrawn: 0,
    }),
  ),
  // The approvals path serves both the inbox and the decided history;
  // the state query parameter picks which side answers.
  http.get("/api/approvals", ({ request }) => {
    const state = new URL(request.url).searchParams.get("state");
    return HttpResponse.json(state === "decided" ? page([]) : page([]));
  }),
  http.post("/api/tickets/:id/actions", () =>
    HttpResponse.json(ticketFixture({ status: "in_review" })),
  ),
  http.get("/api/ticket-types", () => HttpResponse.json(page([]))),

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
  http.get("/api/jobs", () => HttpResponse.json(page([]))),
];
