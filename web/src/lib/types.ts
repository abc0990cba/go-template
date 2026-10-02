// Domain types mirroring the JSON contract in api/openapi/openapi.yaml.
// All timestamps arrive as ISO-8601 strings.

export type { User, Me, SessionInfo } from "@/shared/types/core";
export type { Paged, ListParams, PaginationMode, ViewMode } from "@/shared/api/pagination";

// Re-exported for one release of the migration; import from the owning
// feature (or @/shared/types/core) instead.
export type { User as UserLegacy } from "@/shared/types/core";

export interface OutboxEvent {
  id: string;
  event_type: string;
  aggregate_id: string;
  payload: string;
  created_at: string;
  processed_at?: string | null;
  attempts: number;
}

export interface DeadLetter {
  id: string;
  source: string;
  topic: string;
  payload: string;
  reason: string;
  attempts: number;
  created_at: string;
  replayed_at?: string | null;
}

export interface AuthIdentity {
  user_id: string;
  provider: "password" | "google" | "github" | "facebook" | "twitter";
  provider_subject: string;
  email: string;
  name: string;
  avatar_url: string;
  linked_at: string;
  last_login_at: string;
}

export type JobStatus = "queued" | "running" | "done" | "failed";
export type JobType = "user.enrich_batch" | "user.report_generate";

export interface Job {
  id: string;
  type: string;
  status: JobStatus;
  payload: string;
  error: string;
  created_at: string;
  updated_at: string;
}

export interface Report {
  id: string;
  status: "done" | "failed";
  result: string;
  error: string;
  created_at: string;
  updated_at: string;
}

export interface EnrichmentProfile {
  user_id: string;
  source: string;
  payload: string;
  created_at: string;
}

export interface AnalyticsSummary {
  total_users: number;
  password_identities: number;
  google_identities: number;
  github_identities: number;
  facebook_identities: number;
  twitter_identities: number;
  enrichment_succeeded: number;
  enrichment_failed: number;
  jobs_queued: number;
  jobs_done: number;
  jobs_failed: number;
  events_registered?: number;
  events_logged_in?: number;
  events_oauth_linked?: number;
  events_jobs_queued?: number;
  events_jobs_done?: number;
  events_jobs_failed?: number;
}

export interface EventRecord {
  name: string;
  occurred_at: string;
  user_id?: string;
}

export interface EventDayPoint {
  day: string;
  events: Record<string, number>;
  total: number;
}

export interface OAuthProviderInfo {
  name: string;
  configured: boolean;
}

export type OrgInvitationStatus = "pending" | "accepted" | "revoked" | "expired";

export interface OrgInvitation {
  id: string;
  org_id: string;
  email: string;
  role: string;
  invited_by?: string;
  status: OrgInvitationStatus;
  expires_at: string;
  accepted_at?: string;
  accepted_by?: string;
  created_at: string;
}

export interface Organization {
  id: string;
  name: string;
  slug: string;
  created_at: string;
  updated_at: string;
}

export interface OrgMember {
  org_id: string;
  user_id: string;
  email: string;
  name: string;
  joined_at: string;
}

export interface Role {
  key: string;
  name: string;
  description: string;
  is_system: boolean;
  created_at: string;
}

/** ptype "p": subject/domain/object/action; ptype "g": user/role/domain. */
export interface PolicyRule {
  ptype: "p" | "g";
  subject: string;
  domain: string;
  object?: string;
  action?: string;
}

export interface RbacResource {
  key: string;
  description: string;
  actions: string[];
}

export interface Permission {
  object: string;
  action: string;
}

export interface AuditEntry {
  id: string;
  actor_id: string;
  actor_email: string;
  action: string;
  resource_type: string;
  resource_id?: string;
  domain?: string;
  diff?: string;
  ip?: string;
  request_id?: string;
  created_at: string;
}

export interface SettingDefinition {
  key: string;
  type: "bool" | "int" | "string";
  default: string;
  group: string;
  label: string;
  public: boolean;
  is_flag: boolean;
}

export interface SettingView extends SettingDefinition {
  value: string;
  updated_by?: string;
  updated_at?: string;
}

export interface Notification {
  id: string;
  user_id: string;
  type: string;
  title: string;
  body: string;
  link: string;
  read_at: string | null;
  created_at: string;
}

export interface APIKey {
  id: string;
  user_id: string;
  name: string;
  prefix: string;
  scopes: string[];
  last_used_at: string | null;
  expires_at: string | null;
  revoked_at: string | null;
  created_at: string;
}

export interface WebhookEndpoint {
  id: string;
  url: string;
  events: string[];
  description?: string;
  active: boolean;
  created_at: string;
  updated_at: string;
}

export interface WebhookDelivery {
  id: string;
  endpoint_id: string;
  event: string;
  payload: string;
  status: "pending" | "success" | "failed";
  attempts: number;
  last_response_status: number;
  last_error: string;
  delivered_at: string | null;
  created_at: string;
}

export interface NameCount {
  name: string;
  count: number;
}

export interface DayCount {
  day: string;
  count: number;
}

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
