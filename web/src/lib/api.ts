import { ApiError, http, getActiveOrg, setActiveOrg } from "@/shared/api/core";
import { buildListQuery } from "@/shared/api/pagination";

export { ApiError, buildListQuery, getActiveOrg, setActiveOrg };

import type {
  AnalyticsSummary,
  AuditEntry,
  AuthIdentity,
  EnrichmentProfile,
  EventDayPoint,
  EventRecord,
  Job,
  ListParams,
  APIKey,
  Me,
  Notification,
  OAuthProviderInfo,
  Organization,
  OrgInvitation,
  OrgMember,
  Paged,
  Permission,
  PolicyRule,
  Report,
  Role,
  RbacResource,
  DayCount,
  NameCount,
  SessionInfo,
  SettingView,
  Ticket,
  TicketAction,
  TicketComment,
  TicketDecision,
  TicketDetail,
  TicketEvent,
  TicketType,
  WebhookDelivery,
  WebhookEndpoint,
  User,
  OutboxEvent,
  DeadLetter,
} from "./types";

interface UserWithToken {
  user: User;
  token: string;
}

/** MFA answers the login can hand back instead of a session. */
export type LoginOutcome =
  | { kind: "ok"; user: User; token: string }
  | { kind: "mfa_required"; challenge: string }
  | { kind: "enrollment_required" };

export interface MFAStatus {
  enabled: boolean;
  pending: boolean;
  required: boolean;
}

export interface MFAEnrollment {
  secret: string;
  otpauth_url: string;
}

/** loginOutcome classifies the password step without throwing for the two
 * intentional non-200 answers (the challenge and the enrollment gate). */
async function loginOutcome(email: string, password: string): Promise<LoginOutcome> {
  let response: Response;
  try {
    response = await fetch("/api/auth/login", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
  } catch (cause) {
    throw new ApiError(
      0,
      "network error",
      cause instanceof Error ? cause.message : "network error",
    );
  }
  if (response.ok) {
    const ok = (await response.json()) as UserWithToken;
    return { kind: "ok", user: ok.user, token: ok.token };
  }
  let body: Record<string, unknown> = {};
  try {
    body = (await response.json()) as Record<string, unknown>;
  } catch {
    // Non-JSON error bodies are fine; the status still carries meaning.
  }
  if (body.mfa_required === true && typeof body.challenge === "string") {
    return { kind: "mfa_required", challenge: body.challenge };
  }
  if (body.mfa_enrollment_required === true) return { kind: "enrollment_required" };
  throw new ApiError(
    response.status,
    (body.title as string) ?? "request failed",
    (body.detail as string) ?? "",
  );
}

export const api = {
  register: (email: string, name: string, password: string) =>
    http.post<UserWithToken>("/api/auth/register", { email, name, password }),
  login: async (email: string, password: string) => {
    const outcome = await loginOutcome(email, password);
    if (outcome.kind !== "ok") {
      // The demo picker only signs into MFA-less accounts; reaching this
      // branch means the caller ignored the outcome protocol.
      throw new ApiError(401, "mfa_required", "This account requires a second step");
    }
    return { user: outcome.user, token: outcome.token };
  },
  /** loginWithChallenge runs the password step and reports its outcome. */
  loginStep: loginOutcome,
  mfaChallenge: (challenge: string, code: string) =>
    http.post<UserWithToken>("/api/auth/mfa/challenge", { challenge, code }),
  mfaStatus: () => http.get<MFAStatus>("/api/auth/mfa/status"),
  mfaEnroll: () => http.post<MFAEnrollment>("/api/auth/mfa/enroll"),
  mfaVerify: (code: string) =>
    http.post<{ recovery_codes: string[] }>("/api/auth/mfa/verify", { code }),
  mfaDisable: (password: string, code: string) =>
    http.post<void>("/api/auth/mfa/disable", { password, code }),
  resetUserMFA: (id: string) => http.del<void>(`/api/users/${encodeURIComponent(id)}/mfa`),
  inviteOrgMember: (orgId: string, email: string, role: string) =>
    http.post<OrgInvitation & { token: string }>(
      `/api/orgs/${encodeURIComponent(orgId)}/invitations`,
      {
        email,
        role,
      },
    ),
  orgInvitations: (orgId: string) =>
    http
      .get<Paged<OrgInvitation>>(`/api/orgs/${encodeURIComponent(orgId)}/invitations`)
      .then((p) => p.data),
  revokeOrgInvitation: (orgId: string, invitationId: string) =>
    http.del<void>(
      `/api/orgs/${encodeURIComponent(orgId)}/invitations/${encodeURIComponent(invitationId)}`,
    ),
  myInvitations: () => http.get<Paged<OrgInvitation>>("/api/invitations").then((p) => p.data),
  acceptInvitation: (token: string) =>
    http.post<{ org_id: string; role: string }>("/api/invitations/accept", { token }),
  logout: () => http.post<void>("/api/auth/logout"),
  me: () => http.get<Me>("/api/me"),
  myIdentities: () => http.get<Paged<AuthIdentity>>("/api/me/identities").then((p) => p.data),
  sessions: () => http.get<Paged<SessionInfo>>("/api/sessions").then((p) => p.data),
  logoutAll: () => http.post<void>("/api/profile/logout-all"),
  unlinkProvider: (provider: string) => http.post<void>(`/api/profile/unlink/${provider}`),
  oauthProviders: () =>
    http.get<Paged<OAuthProviderInfo>>("/api/oauth/providers").then((p) => p.data),

  listUsers: (params: Partial<ListParams>) =>
    http.get<Paged<User>>(`/api/users?${buildListQuery(params)}`),
  createUser: (email: string, name: string, external_ref?: string) =>
    http.post<User>("/api/users", { email, name, external_ref }),
  getUser: (id: string) => http.get<User>(`/api/users/${encodeURIComponent(id)}`),
  updateUser: (id: string, email: string, name: string, external_ref?: string) =>
    http.put<User>(`/api/users/${encodeURIComponent(id)}`, { email, name, external_ref }),
  deleteUser: (id: string) => http.del<void>(`/api/users/${encodeURIComponent(id)}`),

  listJobs: (params: Partial<ListParams>) =>
    http.get<Paged<Job>>(`/api/jobs?${buildListQuery(params)}`),
  getJob: (id: string) => http.get<Job>(`/api/jobs/${encodeURIComponent(id)}`),
  processJob: () => http.post<{ processed: boolean }>("/api/jobs/process"),
  enqueueEnrichment: (count: number) => http.post<Job>("/api/jobs/enrichment", { count }),
  listOutbox: (params: Partial<ListParams> & { status?: string }) =>
    http.get<Paged<OutboxEvent> & { pending?: number }>(
      `/api/streams/outbox?${buildListQuery(params)}`,
    ),
  listDeadLetters: (params: Partial<ListParams> & { pending?: boolean }) =>
    http.get<Paged<DeadLetter> & { pending?: number }>(
      `/api/streams/dead-letters?${buildListQuery({ ...params, status: params.pending ? "pending" : undefined })}`,
    ),
  replayDeadLetter: (id: string) =>
    http.post<DeadLetter>(`/api/streams/dead-letters/${encodeURIComponent(id)}/replay`, {}),
  triggerEnrichDemo: (id: string, source: "kafka" | "nats") =>
    http.post<{ published: boolean; source: string; event_id: string; seq: number }>(
      `/api/users/${encodeURIComponent(id)}/enrich-demo`,
      { source },
    ),
  listEnrichmentProfiles: (params: Partial<ListParams>) =>
    http.get<Paged<EnrichmentProfile>>(`/api/enrichment-profiles?${buildListQuery(params)}`),

  listReports: (params: Partial<ListParams>) =>
    http.get<Paged<Report>>(`/api/reports?${buildListQuery(params)}`),
  getReport: (id: string) => http.get<Report>(`/api/reports/${encodeURIComponent(id)}`),
  generateReport: () => http.post<Job>("/api/reports"),

  analyticsSummary: () => http.get<AnalyticsSummary>("/api/analytics/summary"),
  analyticsTimeseries: (days: number) =>
    http
      .get<{ days: number; data: EventDayPoint[] }>(`/api/analytics/timeseries?days=${days}`)
      .then((r) => r.data),
  recentEvents: (limit: number) =>
    http.get<Paged<EventRecord>>(`/api/analytics/events?limit=${limit}`).then((p) => p.data),
  analyticsBreakdown: (by: string, days: number) =>
    http
      .get<{ days: number; data: NameCount[] }>(`/api/analytics/breakdown?by=${by}&days=${days}`)
      .then((r) => r.data),
  analyticsDAU: (days: number) =>
    http
      .get<{ days: number; data: DayCount[] }>(`/api/analytics/dau?days=${days}`)
      .then((r) => r.data),

  rbacCatalog: () =>
    http.get<{ resources: RbacResource[] }>("/api/rbac/catalog").then((r) => r.resources),
  settings: () => http.get<Paged<SettingView>>("/api/settings").then((p) => p.data),
  updateSetting: (key: string, value: string) =>
    http.put<void>(`/api/settings/${encodeURIComponent(key)}`, { value }),
  publicSettings: () => http.get<Record<string, string>>("/api/settings/public"),

  forgotPassword: (email: string) => http.post<void>("/api/auth/forgot-password", { email }),
  resetPassword: (token: string, password: string) =>
    http.post<void>("/api/auth/reset-password", { token, password }),
  verifyEmail: (token: string) => http.post<void>("/api/auth/verify-email", { token }),

  listAPIKeys: () => http.get<Paged<APIKey>>("/api/api-keys").then((p) => p.data),
  createAPIKey: (name: string, scopes: string[]) =>
    http.post<{ key: APIKey; secret: string }>("/api/api-keys", { name, scopes }),
  revokeAPIKey: (id: string) => http.del<void>(`/api/api-keys/${encodeURIComponent(id)}`),

  impersonate: (userId: string) =>
    http.post<{ status: string; user_id: string }>("/api/impersonation", { user_id: userId }),
  stopImpersonation: () => http.post<void>("/api/impersonation/stop"),

  listWebhooks: () => http.get<Paged<WebhookEndpoint>>("/api/webhooks").then((p) => p.data),
  createWebhook: (url: string, events: string[]) =>
    http.post<{ id: string; url: string; secret: string; events: string[]; active: boolean }>(
      "/api/webhooks",
      { url, events },
    ),
  updateWebhook: (
    id: string,
    url: string,
    events: string[],
    description: string,
    active: boolean,
  ) =>
    http.put<void>(`/api/webhooks/${encodeURIComponent(id)}`, { url, events, description, active }),
  deleteWebhook: (id: string) => http.del<void>(`/api/webhooks/${encodeURIComponent(id)}`),
  webhookDeliveries: (id: string) =>
    http
      .get<Paged<WebhookDelivery>>(`/api/webhooks/${encodeURIComponent(id)}/deliveries`)
      .then((p) => p.data),
  testWebhook: (id: string) =>
    http.post<{ status: string }>(`/api/webhooks/${encodeURIComponent(id)}/test`),

  notifications: (limit = 25) =>
    http.get<Paged<Notification>>(`/api/notifications?limit=${limit}`).then((p) => p.data),
  unreadCount: () => http.get<{ count: number }>("/api/notifications/unread-count"),
  markNotificationRead: (id: string) =>
    http.post<void>(`/api/notifications/${encodeURIComponent(id)}/read`),
  markAllNotificationsRead: () => http.post<void>("/api/notifications/read-all"),

  listAudit: (
    params: Partial<ListParams> & {
      actor?: string;
      action?: string;
      resource?: string;
      resource_id?: string;
    },
  ) => {
    const base = buildListQuery(params);
    const extra = new URLSearchParams();
    if (params.actor) extra.set("actor", params.actor);
    if (params.action) extra.set("action", params.action);
    if (params.resource) extra.set("resource", params.resource);
    if (params.resource_id) extra.set("resource_id", params.resource_id);
    const suffix = extra.toString();
    return http.get<Paged<AuditEntry>>(`/api/audit?${suffix ? base + "&" + suffix : base}`);
  },
  /** One user's history: audit records where they are actor or subject. */
  userActivity: (id: string, limit = 25, offset = 0) =>
    http.get<Paged<AuditEntry>>(
      `/api/users/${encodeURIComponent(id)}/activity?limit=${limit}&offset=${offset}`,
    ),
  listRoles: () => http.get<Paged<Role>>("/api/roles").then((p) => p.data),
  createRole: (key: string, name: string, description: string) =>
    http.post<Role>("/api/roles", { key, name, description }),
  updateRole: (key: string, name: string, description: string) =>
    http.put<void>(`/api/roles/${encodeURIComponent(key)}`, { name, description }),
  deleteRole: (key: string) => http.del<void>(`/api/roles/${encodeURIComponent(key)}`),
  rolePermissions: (key: string) =>
    http
      .get<Paged<PolicyRule>>(`/api/roles/${encodeURIComponent(key)}/permissions`)
      .then((p) => p.data),
  setRolePermissions: (key: string, domain: string, permissions: Permission[]) =>
    http.put<void>(`/api/roles/${encodeURIComponent(key)}/permissions`, { domain, permissions }),

  listPolicies: (ptype: "p" | "g") =>
    http.get<Paged<PolicyRule>>(`/api/policies?ptype=${ptype}`).then((p) => p.data),
  addPolicy: (rule: PolicyRule) => http.post<void>("/api/policies", rule),
  removePolicy: (rule: PolicyRule) => http.del<void>("/api/policies", rule),

  userRoles: (id: string) =>
    http.get<Paged<PolicyRule>>(`/api/users/${encodeURIComponent(id)}/roles`).then((p) => p.data),
  assignUserRole: (id: string, role: string, domain: string) =>
    http.post<void>(`/api/users/${encodeURIComponent(id)}/roles`, { role, domain }),
  unassignUserRole: (id: string, role: string, domain: string) =>
    http.del<void>(`/api/users/${encodeURIComponent(id)}/roles`, { role, domain }),

  listOrgs: (params: Partial<ListParams>) =>
    http.get<Paged<Organization>>(`/api/orgs?${buildListQuery(params)}`),
  createOrg: (name: string, slug: string) => http.post<Organization>("/api/orgs", { name, slug }),
  getOrg: (id: string) => http.get<Organization>(`/api/orgs/${encodeURIComponent(id)}`),
  updateOrg: (id: string, name: string, slug: string) =>
    http.put<void>(`/api/orgs/${encodeURIComponent(id)}`, { name, slug }),
  deleteOrg: (id: string) => http.del<void>(`/api/orgs/${encodeURIComponent(id)}`),
  orgMembers: (id: string) =>
    http.get<Paged<OrgMember>>(`/api/orgs/${encodeURIComponent(id)}/members`).then((p) => p.data),
  addOrgMember: (id: string, userId: string) =>
    http.post<void>(`/api/orgs/${encodeURIComponent(id)}/members`, { user_id: userId }),
  removeOrgMember: (id: string, userId: string) =>
    http.del<void>(`/api/orgs/${encodeURIComponent(id)}/members/${encodeURIComponent(userId)}`),

  // Tickets: requesters work their own list, approvers work the inbox.
  listTickets: (params: Partial<ListParams> & { scope?: "my" | "all"; priority?: string }) => {
    const extra = new URLSearchParams();
    if (params.scope) extra.set("scope", params.scope);
    if (params.priority) extra.set("priority", params.priority);
    const suffix = extra.toString();
    const base = buildListQuery(params);
    return http.get<Paged<Ticket>>(`/api/tickets?${suffix ? base + "&" + suffix : base}`);
  },
  ticketCounts: (scope: "my" | "all" = "my") =>
    http.get<Record<string, number>>(`/api/tickets/counts?scope=${scope}`),
  createTicket: (typeId: string, title: string, description: string, priority: string) =>
    http.post<Ticket>("/api/tickets", { type_id: typeId, title, description, priority }),
  getTicket: (id: string) => http.get<TicketDetail>(`/api/tickets/${encodeURIComponent(id)}`),
  updateTicket: (id: string, title: string, description: string, priority: string) =>
    http.patch<Ticket>(`/api/tickets/${encodeURIComponent(id)}`, { title, description, priority }),
  deleteTicket: (id: string) => http.del<void>(`/api/tickets/${encodeURIComponent(id)}`),
  ticketAction: (id: string, action: TicketAction, comment = "") =>
    http.post<Ticket>(`/api/tickets/${encodeURIComponent(id)}/actions`, { action, comment }),
  ticketComments: (id: string, limit = 25) =>
    http
      .get<Paged<TicketComment>>(`/api/tickets/${encodeURIComponent(id)}/comments?limit=${limit}`)
      .then((p) => p.data),
  addTicketComment: (id: string, body: string) =>
    http.post<TicketComment>(`/api/tickets/${encodeURIComponent(id)}/comments`, { body }),
  ticketEvents: (id: string, limit = 50) =>
    http
      .get<Paged<TicketEvent>>(`/api/tickets/${encodeURIComponent(id)}/events?limit=${limit}`)
      .then((p) => p.data),

  approvals: (params: Partial<ListParams>) =>
    http.get<Paged<Ticket>>(`/api/approvals?${buildListQuery(params)}`),
  decidedApprovals: (limit = 25, offset = 0) =>
    http.get<Paged<TicketDecision>>(`/api/approvals?state=decided&limit=${limit}&offset=${offset}`),
  ticketTypes: () => http.get<Paged<TicketType>>("/api/ticket-types").then((p) => p.data),
  createTicketType: (
    key: string,
    name: string,
    description: string,
    steps: { name: string; approver_role: string }[],
  ) => http.post<TicketType>("/api/ticket-types", { key, name, description, steps }),
  updateTicketType: (
    id: string,
    patchBody: {
      name?: string;
      description?: string;
      steps?: { name: string; approver_role: string }[];
      active?: boolean;
    },
  ) => http.patch<TicketType>(`/api/ticket-types/${encodeURIComponent(id)}`, patchBody),
  deleteTicketType: (id: string) => http.del<void>(`/api/ticket-types/${encodeURIComponent(id)}`),
};
