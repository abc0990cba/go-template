import { api, unwrap, unwrapWithETag } from "@/shared/api/core";
import type { Paged, ListParams } from "@/shared/api/pagination";
import type { operations } from "@/shared/api/schema.gen";
import type {
  Ticket,
  TicketAction,
  TicketComment,
  TicketDetail,
  TicketEvent,
  TicketPriority,
  TicketStep,
  TicketType,
} from "./types";

/** Endpoints of the tickets slice: the requester flow plus the type
 * catalog that defines the approval chains. Every call goes through the
 * typed client, so paths, bodies, and responses are checked against the
 * OpenAPI contract at compile time. */

type ListTicketsQuery = operations["listTickets"]["parameters"]["query"];

/** The shared list vocabulary plus the tickets scope tab. The URL keeps
 * status/priority as free strings; the backend 400s anything outside the
 * spec enums, and the table filters only offer valid values. */
export type TicketsListParams = Partial<ListParams> & {
  scope?: "my" | "all";
  priority?: string;
};

export function listTickets(params: TicketsListParams): Promise<Paged<Ticket>> {
  return unwrap(api.GET("/api/tickets", { params: { query: params as ListTicketsQuery } }));
}

export function ticketCounts(scope: "my" | "all" = "my"): Promise<Record<string, number>> {
  return unwrap(api.GET("/api/tickets/counts", { params: { query: { scope } } }));
}

export function createTicket(
  typeId: string,
  title: string,
  description: string,
  priority: TicketPriority,
): Promise<Ticket> {
  return unwrap(
    api.POST("/api/tickets", { body: { type_id: typeId, title, description, priority } }),
  );
}

export function getTicket(id: string): Promise<TicketDetail> {
  return unwrap(api.GET("/api/tickets/{id}", { params: { path: { id } } }));
}

/** getTicketWithETag pairs the detail with its ETag for optimistic writes. */
export function getTicketWithETag(id: string): Promise<{ data: TicketDetail; etag: string }> {
  return unwrapWithETag(api.GET("/api/tickets/{id}", { params: { path: { id } } }));
}

export function updateTicket(
  id: string,
  title: string,
  description: string,
  priority: TicketPriority,
  etag?: string,
): Promise<Ticket> {
  return unwrap(
    api.PATCH("/api/tickets/{id}", {
      params: { path: { id }, header: etag ? { "If-Match": etag } : undefined },
      body: { title, description, priority },
    }),
  );
}

export function deleteTicket(id: string): Promise<void> {
  return unwrap(api.DELETE("/api/tickets/{id}", { params: { path: { id } } }));
}

export type TicketActionName = TicketAction;

export function ticketAction(id: string, action: TicketActionName, comment = ""): Promise<Ticket> {
  return unwrap(
    api.POST("/api/tickets/{id}/actions", {
      params: { path: { id } },
      body: comment ? { action, comment } : { action },
    }),
  );
}

export function ticketComments(id: string, limit = 25): Promise<TicketComment[]> {
  return unwrap(
    api.GET("/api/tickets/{id}/comments", { params: { path: { id }, query: { limit } } }),
  ).then((p) => p.data);
}

export function addTicketComment(id: string, body: string): Promise<TicketComment> {
  return unwrap(
    api.POST("/api/tickets/{id}/comments", { params: { path: { id } }, body: { body } }),
  );
}

export function ticketEvents(id: string, limit = 50): Promise<TicketEvent[]> {
  return unwrap(
    api.GET("/api/tickets/{id}/events", { params: { path: { id }, query: { limit } } }),
  ).then((p) => p.data);
}

export function ticketTypes(): Promise<TicketType[]> {
  return unwrap(api.GET("/api/ticket-types")).then((p) => p.data);
}

export function createTicketType(
  key: string,
  name: string,
  description: string,
  steps: TicketStep[],
): Promise<TicketType> {
  return unwrap(api.POST("/api/ticket-types", { body: { key, name, description, steps } }));
}

export function updateTicketType(
  id: string,
  patchBody: {
    name?: string;
    description?: string;
    steps?: TicketStep[];
    active?: boolean;
  },
): Promise<TicketType> {
  return unwrap(api.PATCH("/api/ticket-types/{id}", { params: { path: { id } }, body: patchBody }));
}

export function deleteTicketType(id: string): Promise<void> {
  return unwrap(api.DELETE("/api/ticket-types/{id}", { params: { path: { id } } }));
}
