import { http } from "@/shared/api/core";
import { buildListQuery, type Paged } from "@/shared/api/pagination";
import type { ListParams } from "@/shared/api/pagination";
import type { Ticket, TicketComment, TicketDetail, TicketEvent, TicketType } from "./types";

/** Endpoints of the tickets slice: the requester flow plus the type
 * catalog that defines the approval chains. */
export type TicketsListParams = Partial<ListParams> & {
  scope?: "my" | "all";
  priority?: string;
};

export function listTickets(params: TicketsListParams): Promise<Paged<Ticket>> {
  const extra = new URLSearchParams();
  if (params.scope) extra.set("scope", params.scope);
  if (params.priority) extra.set("priority", params.priority);
  const suffix = extra.toString();
  const base = buildListQuery(params);
  return http.get<Paged<Ticket>>(`/api/tickets?${suffix ? base + "&" + suffix : base}`);
}

export function ticketCounts(scope: "my" | "all" = "my"): Promise<Record<string, number>> {
  return http.get(`/api/tickets/counts?scope=${scope}`);
}

export function createTicket(
  typeId: string,
  title: string,
  description: string,
  priority: string,
): Promise<Ticket> {
  return http.post("/api/tickets", { type_id: typeId, title, description, priority });
}

export function getTicket(id: string): Promise<TicketDetail> {
  return http.get<TicketDetail>(`/api/tickets/${encodeURIComponent(id)}`);
}

export function updateTicket(
  id: string,
  title: string,
  description: string,
  priority: string,
): Promise<Ticket> {
  return http.patch(`/api/tickets/${encodeURIComponent(id)}`, { title, description, priority });
}

export function deleteTicket(id: string): Promise<void> {
  return http.del<void>(`/api/tickets/${encodeURIComponent(id)}`);
}

export type TicketActionName = "submit" | "approve" | "reject" | "withdraw" | "complete" | "reopen";

export function ticketAction(id: string, action: TicketActionName, comment = ""): Promise<Ticket> {
  return http.post<Ticket>(`/api/tickets/${encodeURIComponent(id)}/actions`, { action, comment });
}

export function ticketComments(id: string, limit = 25): Promise<TicketComment[]> {
  return http
    .get<Paged<TicketComment>>(`/api/tickets/${encodeURIComponent(id)}/comments?limit=${limit}`)
    .then((p) => p.data);
}

export function addTicketComment(id: string, body: string): Promise<TicketComment> {
  return http.post<TicketComment>(`/api/tickets/${encodeURIComponent(id)}/comments`, { body });
}

export function ticketEvents(id: string, limit = 50): Promise<TicketEvent[]> {
  return http
    .get<Paged<TicketEvent>>(`/api/tickets/${encodeURIComponent(id)}/events?limit=${limit}`)
    .then((p) => p.data);
}

export function ticketTypes(): Promise<TicketType[]> {
  return http.get<Paged<TicketType>>("/api/ticket-types").then((p) => p.data);
}

export function createTicketType(
  key: string,
  name: string,
  description: string,
  steps: { name: string; approver_role: string }[],
): Promise<TicketType> {
  return http.post<TicketType>("/api/ticket-types", { key, name, description, steps });
}

export function updateTicketType(
  id: string,
  patchBody: {
    name?: string;
    description?: string;
    steps?: { name: string; approver_role: string }[];
    active?: boolean;
  },
): Promise<TicketType> {
  return http.patch<TicketType>(`/api/ticket-types/${encodeURIComponent(id)}`, patchBody);
}

export function deleteTicketType(id: string): Promise<void> {
  return http.del<void>(`/api/ticket-types/${encodeURIComponent(id)}`);
}
