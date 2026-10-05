import { HttpResponse } from "msw";
import type { Paged } from "@/shared/api/pagination";
import type { StoredFile } from "@/features/files/api";
import type { Ticket, TicketDecision, TicketType } from "@/features/tickets";
import type { Me } from "@/shared/types/core";

/** Shared fixtures + response helpers for the MSW handler modules. */

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

/** A minimal valid stored-file record for attachment endpoints. */
export function fileFixture(overrides: Partial<StoredFile> = {}): StoredFile {
  return {
    id: "f-1",
    owner_id: "u-test-1",
    filename: "evidence.bin",
    content_type: "application/octet-stream",
    size_bytes: 11,
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}
