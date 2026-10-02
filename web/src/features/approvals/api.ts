import { api, unwrap } from "@/shared/api/core";
import type { ListParams, Paged } from "@/shared/api/pagination";
import type { operations } from "@/shared/api/schema.gen";
import type { Ticket, TicketDecision } from "@/features/tickets";

/** Endpoints of the approvals slice: the role-scoped decision inbox. Every
 * call goes through the typed client, so paths, queries, and responses are
 * checked against the OpenAPI contract at compile time. */

type ListApprovalsQuery = operations["listApprovals"]["parameters"]["query"];

/** The shared list vocabulary against the inbox. The URL keeps the params
 * as free strings (as the tickets list does); the query is cast because the
 * shared params carry a few filters this endpoint does not declare. */
export function approvals(params: Partial<ListParams>): Promise<Paged<Ticket>> {
  return unwrap(
    api.GET("/api/approvals", { params: { query: params as ListApprovalsQuery } }),
  ) as Promise<Paged<Ticket>>;
}

/** The decision history: state=decided makes the endpoint answer
 * TicketDecisionList instead of the inbox's TicketList — a wire union the
 * spec declares for the single path, narrowed here to the shape the caller
 * asked for. */
export function decidedApprovals(limit = 25, offset = 0): Promise<Paged<TicketDecision>> {
  return unwrap(
    api.GET("/api/approvals", { params: { query: { state: "decided", limit, offset } } }),
  ) as Promise<Paged<TicketDecision>>;
}
