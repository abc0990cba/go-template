// --- Tickets ------------------------------------------------------------

export type TicketStatus =
  | "draft"
  | "in_review"
  | "approved"
  | "completed"
  | "rejected"
  | "withdrawn";

export type TicketPriority = "low" | "medium" | "high" | "urgent";

export type TicketAction = "submit" | "approve" | "reject" | "withdraw" | "complete" | "reopen";

/** One approval step of a ticket type: the named role decides it in order. */
export interface TicketStep {
  name: string;
  approver_role: string;
}

export interface TicketType {
  id: string;
  org_id?: string;
  key: string;
  name: string;
  description: string;
  steps: TicketStep[];
  active: boolean;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

export interface Ticket {
  id: string;
  number: number;
  org_id?: string;
  type_id: string;
  type_key: string;
  title: string;
  description: string;
  priority: TicketPriority;
  status: TicketStatus;
  current_step: number;
  created_by: string;
  decided_by?: string;
  decided_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface TicketApproval {
  id: string;
  ticket_id: string;
  step_index: number;
  name: string;
  approver_role: string;
  status: "pending" | "approved" | "rejected" | "skipped";
  decided_by?: string;
  decided_at: string | null;
  comment?: string;
  created_at: string;
}

export interface TicketDetail {
  ticket: Ticket;
  approvals: TicketApproval[];
}

export interface TicketComment {
  id: string;
  ticket_id: string;
  author_id: string;
  body: string;
  created_at: string;
}

export interface TicketEvent {
  id: string;
  ticket_id: string;
  actor_id: string;
  action: string;
  detail?: string;
  created_at: string;
}

export interface TicketDecision {
  approval: TicketApproval;
  ticket: Ticket;
}
