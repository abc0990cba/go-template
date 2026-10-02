import type { components, operations } from "@/shared/api/schema.gen";

/**
 * Wire shapes of the tickets slice, derived from the generated API contract
 * (schema.gen.ts ← api/openapi/openapi.yaml). To change a shape, change the
 * spec and run `npm run gen:api` — these types are never hand-edited.
 */

type Schemas = components["schemas"];

export type TicketStatus = Schemas["Ticket"]["status"];

export type TicketPriority = Schemas["Ticket"]["priority"];

export type TicketAction =
  operations["ticketAction"]["requestBody"]["content"]["application/json"]["action"];

/** One approval step of a ticket type: the named role decides it in order. */
export type TicketStep = Schemas["TicketStep"];

export type TicketType = Schemas["TicketType"];

export type Ticket = Schemas["Ticket"];

export type TicketApproval = Schemas["TicketApproval"];

export type TicketDetail = Schemas["TicketDetail"];

export type TicketComment = Schemas["TicketComment"];

export type TicketEvent = Schemas["TicketEvent"];

export type TicketDecision = Schemas["TicketDecision"];
