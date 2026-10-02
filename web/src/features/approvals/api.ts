import { http } from "@/shared/api/core";
import { buildListQuery, type Paged } from "@/shared/api/pagination";
import type { ListParams } from "@/shared/api/pagination";
import type { Ticket, TicketDecision } from "@/features/tickets";

/** Endpoints of the approvals slice: the role-scoped decision inbox. */
export function approvals(params: Partial<ListParams>): Promise<Paged<Ticket>> {
  return http.get<Paged<Ticket>>(`/api/approvals?${buildListQuery(params)}`);
}

export function decidedApprovals(limit = 25, offset = 0): Promise<Paged<TicketDecision>> {
  return http.get<Paged<TicketDecision>>(
    `/api/approvals?state=decided&limit=${limit}&offset=${offset}`,
  );
}
