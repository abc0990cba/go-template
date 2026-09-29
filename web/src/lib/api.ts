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

/**
 * ApiError carries the problem+json details of a failed API call.
 * Network failures surface as ApiError with status 0.
 */
export class ApiError extends Error {
  readonly status: number;
  readonly title: string;
  readonly detail: string;

  constructor(status: number, title: string, detail: string) {
    super(detail || title);
    this.name = "ApiError";
    this.status = status;
    this.title = title;
    this.detail = detail;
  }
}

interface ProblemBody {
  title?: string;
  detail?: string;
}

const ORG_HEADER_KEY = "app.activeOrg";

/**
 * The active organization id (Casbin domain) sent as X-Org-Id with every
 * request; empty string means the platform scope. Persisted so a reload
 * keeps the admin inside the tenant they switched to.
 */
export function getActiveOrg(): string {
  try {
    return localStorage.getItem(ORG_HEADER_KEY) ?? "";
  } catch {
    return "";
  }
}

export function setActiveOrg(orgId: string): void {
  try {
    if (orgId) localStorage.setItem(ORG_HEADER_KEY, orgId);
    else localStorage.removeItem(ORG_HEADER_KEY);
  } catch {
    // Private mode: the header just lives for this page session.
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const orgId = getActiveOrg();
  let response: Response;
  try {
    response = await fetch(path, {
      credentials: "include",
      ...init,
      headers: {
        ...(init?.body === undefined ? {} : { "Content-Type": "application/json" }),
        ...(orgId ? { "X-Org-Id": orgId } : {}),
        ...init?.headers,
      },
    } satisfies RequestInit);
  } catch (cause) {
    throw new ApiError(
      0,
      "network error",
      cause instanceof Error ? cause.message : "network error",
    );
  }
  if (!response.ok) {
    let problem: ProblemBody = {};
    try {
      problem = (await response.json()) satisfies ProblemBody;
    } catch {
      // Non-JSON error bodies are fine; the status still carries meaning.
    }
    throw new ApiError(response.status, problem.title ?? "request failed", problem.detail ?? "");
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

function get<T>(path: string): Promise<T> {
  return request<T>(path);
}

function post<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: "POST",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

function put<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, { method: "PUT", body: JSON.stringify(body) });
}

function patch<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, { method: "PATCH", body: JSON.stringify(body) });
}

function del<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: "DELETE",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

/** buildListQuery renders the shared list vocabulary into a URL query string. */
export function buildListQuery(params: Partial<ListParams>): string {
  const search = new URLSearchParams();
  search.set("mode", params.mode ?? "offset");
  search.set("limit", String(params.limit ?? 10));
  if (params.mode === "offset" && (params.offset ?? 0) > 0) {
    search.set("offset", String(params.offset));
  }
  if (params.mode !== "offset" && params.after) {
    search.set("after", params.after);
  }
  if (params.q) search.set("q", params.q);
  if (params.status) search.set("status", params.status);
  if (params.type) search.set("type", params.type);
  if (params.user_id) search.set("user_id", params.user_id);
  return search.toString();
}

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
    throw new ApiError(0, "network error", cause instanceof Error ? cause.message : "network error");
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
    post<UserWithToken>("/api/auth/register", { email, name, password }),
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
    post<UserWithToken>("/api/auth/mfa/challenge", { challenge, code }),
  mfaStatus: () => get<MFAStatus>("/api/auth/mfa/status"),
  mfaEnroll: () => post<MFAEnrollment>("/api/auth/mfa/enroll"),
  mfaVerify: (code: string) => post<{ recovery_codes: string[] }>("/api/auth/mfa/verify", { code }),
  mfaDisable: (password: string, code: string) =>
    post<void>("/api/auth/mfa/disable", { password, code }),
  resetUserMFA: (id: string) => del<void>(`/api/users/${encodeURIComponent(id)}/mfa`),
  logout: () => post<void>("/api/auth/logout"),
  me: () => get<Me>("/api/me"),
  myIdentities: () => get<Paged<AuthIdentity>>("/api/me/identities").then((p) => p.data),
  sessions: () => get<Paged<SessionInfo>>("/api/sessions").then((p) => p.data),
  logoutAll: () => post<void>("/api/profile/logout-all"),
  unlinkProvider: (provider: string) => post<void>(`/api/profile/unlink/${provider}`),
  oauthProviders: () => get<Paged<OAuthProviderInfo>>("/api/oauth/providers").then((p) => p.data),

  listUsers: (params: Partial<ListParams>) =>
    get<Paged<User>>(`/api/users?${buildListQuery(params)}`),
  createUser: (email: string, name: string, external_ref?: string) =>
    post<User>("/api/users", { email, name, external_ref }),
  getUser: (id: string) => get<User>(`/api/users/${encodeURIComponent(id)}`),
  updateUser: (id: string, email: string, name: string, external_ref?: string) =>
    put<User>(`/api/users/${encodeURIComponent(id)}`, { email, name, external_ref }),
  deleteUser: (id: string) => del<void>(`/api/users/${encodeURIComponent(id)}`),

  listJobs: (params: Partial<ListParams>) => get<Paged<Job>>(`/api/jobs?${buildListQuery(params)}`),
  getJob: (id: string) => get<Job>(`/api/jobs/${encodeURIComponent(id)}`),
  processJob: () => post<{ processed: boolean }>("/api/jobs/process"),
  enqueueEnrichment: (count: number) => post<Job>("/api/jobs/enrichment", { count }),
  listOutbox: (params: Partial<ListParams> & { status?: string }) =>
    get<Paged<OutboxEvent> & { pending?: number }>(`/api/streams/outbox?${buildListQuery(params)}`),
  listDeadLetters: (params: Partial<ListParams> & { pending?: boolean }) =>
    get<Paged<DeadLetter> & { pending?: number }>(
      `/api/streams/dead-letters?${buildListQuery({ ...params, status: params.pending ? "pending" : undefined })}`,
    ),
  replayDeadLetter: (id: string) =>
    post<DeadLetter>(`/api/streams/dead-letters/${encodeURIComponent(id)}/replay`, {}),
  triggerEnrichDemo: (id: string, source: "kafka" | "nats") =>
    post<{ published: boolean; source: string; event_id: string; seq: number }>(
      `/api/users/${encodeURIComponent(id)}/enrich-demo`,
      { source },
    ),
  listEnrichmentProfiles: (params: Partial<ListParams>) =>
    get<Paged<EnrichmentProfile>>(`/api/enrichment-profiles?${buildListQuery(params)}`),

  listReports: (params: Partial<ListParams>) =>
    get<Paged<Report>>(`/api/reports?${buildListQuery(params)}`),
  getReport: (id: string) => get<Report>(`/api/reports/${encodeURIComponent(id)}`),
  generateReport: () => post<Job>("/api/reports"),

  analyticsSummary: () => get<AnalyticsSummary>("/api/analytics/summary"),
  analyticsTimeseries: (days: number) =>
    get<{ days: number; data: EventDayPoint[] }>(`/api/analytics/timeseries?days=${days}`).then(
      (r) => r.data,
    ),
  recentEvents: (limit: number) =>
    get<Paged<EventRecord>>(`/api/analytics/events?limit=${limit}`).then((p) => p.data),
  analyticsBreakdown: (by: string, days: number) =>
    get<{ days: number; data: NameCount[] }>(`/api/analytics/breakdown?by=${by}&days=${days}`).then(
      (r) => r.data,
    ),
  analyticsDAU: (days: number) =>
    get<{ days: number; data: DayCount[] }>(`/api/analytics/dau?days=${days}`).then((r) => r.data),

  rbacCatalog: () =>
    get<{ resources: RbacResource[] }>("/api/rbac/catalog").then((r) => r.resources),
  settings: () => get<Paged<SettingView>>("/api/settings").then((p) => p.data),
  updateSetting: (key: string, value: string) =>
    put<void>(`/api/settings/${encodeURIComponent(key)}`, { value }),
  publicSettings: () => get<Record<string, string>>("/api/settings/public"),

  forgotPassword: (email: string) => post<void>("/api/auth/forgot-password", { email }),
  resetPassword: (token: string, password: string) =>
    post<void>("/api/auth/reset-password", { token, password }),
  verifyEmail: (token: string) => post<void>("/api/auth/verify-email", { token }),

  listAPIKeys: () => get<Paged<APIKey>>("/api/api-keys").then((p) => p.data),
  createAPIKey: (name: string, scopes: string[]) =>
    post<{ key: APIKey; secret: string }>("/api/api-keys", { name, scopes }),
  revokeAPIKey: (id: string) => del<void>(`/api/api-keys/${encodeURIComponent(id)}`),

  impersonate: (userId: string) =>
    post<{ status: string; user_id: string }>("/api/impersonation", { user_id: userId }),
  stopImpersonation: () => post<void>("/api/impersonation/stop"),

  listWebhooks: () => get<Paged<WebhookEndpoint>>("/api/webhooks").then((p) => p.data),
  createWebhook: (url: string, events: string[]) =>
    post<{ id: string; url: string; secret: string; events: string[]; active: boolean }>(
      "/api/webhooks",
      { url, events },
    ),
  updateWebhook: (
    id: string,
    url: string,
    events: string[],
    description: string,
    active: boolean,
  ) => put<void>(`/api/webhooks/${encodeURIComponent(id)}`, { url, events, description, active }),
  deleteWebhook: (id: string) => del<void>(`/api/webhooks/${encodeURIComponent(id)}`),
  webhookDeliveries: (id: string) =>
    get<Paged<WebhookDelivery>>(`/api/webhooks/${encodeURIComponent(id)}/deliveries`).then(
      (p) => p.data,
    ),
  testWebhook: (id: string) =>
    post<{ status: string }>(`/api/webhooks/${encodeURIComponent(id)}/test`),

  notifications: (limit = 25) =>
    get<Paged<Notification>>(`/api/notifications?limit=${limit}`).then((p) => p.data),
  unreadCount: () => get<{ count: number }>("/api/notifications/unread-count"),
  markNotificationRead: (id: string) =>
    post<void>(`/api/notifications/${encodeURIComponent(id)}/read`),
  markAllNotificationsRead: () => post<void>("/api/notifications/read-all"),

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
    return get<Paged<AuditEntry>>(`/api/audit?${suffix ? base + "&" + suffix : base}`);
  },
  /** One user's history: audit records where they are actor or subject. */
  userActivity: (id: string, limit = 25, offset = 0) =>
    get<Paged<AuditEntry>>(
      `/api/users/${encodeURIComponent(id)}/activity?limit=${limit}&offset=${offset}`,
    ),
  listRoles: () => get<Paged<Role>>("/api/roles").then((p) => p.data),
  createRole: (key: string, name: string, description: string) =>
    post<Role>("/api/roles", { key, name, description }),
  updateRole: (key: string, name: string, description: string) =>
    put<void>(`/api/roles/${encodeURIComponent(key)}`, { name, description }),
  deleteRole: (key: string) => del<void>(`/api/roles/${encodeURIComponent(key)}`),
  rolePermissions: (key: string) =>
    get<Paged<PolicyRule>>(`/api/roles/${encodeURIComponent(key)}/permissions`).then((p) => p.data),
  setRolePermissions: (key: string, domain: string, permissions: Permission[]) =>
    put<void>(`/api/roles/${encodeURIComponent(key)}/permissions`, { domain, permissions }),

  listPolicies: (ptype: "p" | "g") =>
    get<Paged<PolicyRule>>(`/api/policies?ptype=${ptype}`).then((p) => p.data),
  addPolicy: (rule: PolicyRule) => post<void>("/api/policies", rule),
  removePolicy: (rule: PolicyRule) => del<void>("/api/policies", rule),

  userRoles: (id: string) =>
    get<Paged<PolicyRule>>(`/api/users/${encodeURIComponent(id)}/roles`).then((p) => p.data),
  assignUserRole: (id: string, role: string, domain: string) =>
    post<void>(`/api/users/${encodeURIComponent(id)}/roles`, { role, domain }),
  unassignUserRole: (id: string, role: string, domain: string) =>
    del<void>(`/api/users/${encodeURIComponent(id)}/roles`, { role, domain }),

  listOrgs: (params: Partial<ListParams>) =>
    get<Paged<Organization>>(`/api/orgs?${buildListQuery(params)}`),
  createOrg: (name: string, slug: string) => post<Organization>("/api/orgs", { name, slug }),
  getOrg: (id: string) => get<Organization>(`/api/orgs/${encodeURIComponent(id)}`),
  updateOrg: (id: string, name: string, slug: string) =>
    put<void>(`/api/orgs/${encodeURIComponent(id)}`, { name, slug }),
  deleteOrg: (id: string) => del<void>(`/api/orgs/${encodeURIComponent(id)}`),
  orgMembers: (id: string) =>
    get<Paged<OrgMember>>(`/api/orgs/${encodeURIComponent(id)}/members`).then((p) => p.data),
  addOrgMember: (id: string, userId: string) =>
    post<void>(`/api/orgs/${encodeURIComponent(id)}/members`, { user_id: userId }),
  removeOrgMember: (id: string, userId: string) =>
    del<void>(`/api/orgs/${encodeURIComponent(id)}/members/${encodeURIComponent(userId)}`),

  // Tickets: requesters work their own list, approvers work the inbox.
  listTickets: (params: Partial<ListParams> & { scope?: "my" | "all"; priority?: string }) => {
    const extra = new URLSearchParams();
    if (params.scope) extra.set("scope", params.scope);
    if (params.priority) extra.set("priority", params.priority);
    const suffix = extra.toString();
    const base = buildListQuery(params);
    return get<Paged<Ticket>>(`/api/tickets?${suffix ? base + "&" + suffix : base}`);
  },
  ticketCounts: (scope: "my" | "all" = "my") =>
    get<Record<string, number>>(`/api/tickets/counts?scope=${scope}`),
  createTicket: (typeId: string, title: string, description: string, priority: string) =>
    post<Ticket>("/api/tickets", { type_id: typeId, title, description, priority }),
  getTicket: (id: string) => get<TicketDetail>(`/api/tickets/${encodeURIComponent(id)}`),
  updateTicket: (id: string, title: string, description: string, priority: string) =>
    patch<Ticket>(`/api/tickets/${encodeURIComponent(id)}`, { title, description, priority }),
  deleteTicket: (id: string) => del<void>(`/api/tickets/${encodeURIComponent(id)}`),
  ticketAction: (id: string, action: TicketAction, comment = "") =>
    post<Ticket>(`/api/tickets/${encodeURIComponent(id)}/actions`, { action, comment }),
  ticketComments: (id: string, limit = 25) =>
    get<Paged<TicketComment>>(
      `/api/tickets/${encodeURIComponent(id)}/comments?limit=${limit}`,
    ).then((p) => p.data),
  addTicketComment: (id: string, body: string) =>
    post<TicketComment>(`/api/tickets/${encodeURIComponent(id)}/comments`, { body }),
  ticketEvents: (id: string, limit = 50) =>
    get<Paged<TicketEvent>>(`/api/tickets/${encodeURIComponent(id)}/events?limit=${limit}`).then(
      (p) => p.data,
    ),

  approvals: (params: Partial<ListParams>) =>
    get<Paged<Ticket>>(`/api/approvals?${buildListQuery(params)}`),
  decidedApprovals: (limit = 25, offset = 0) =>
    get<Paged<TicketDecision>>(`/api/approvals?state=decided&limit=${limit}&offset=${offset}`),
  ticketTypes: () => get<Paged<TicketType>>("/api/ticket-types").then((p) => p.data),
  createTicketType: (
    key: string,
    name: string,
    description: string,
    steps: { name: string; approver_role: string }[],
  ) => post<TicketType>("/api/ticket-types", { key, name, description, steps }),
  updateTicketType: (
    id: string,
    patchBody: {
      name?: string;
      description?: string;
      steps?: { name: string; approver_role: string }[];
      active?: boolean;
    },
  ) => patch<TicketType>(`/api/ticket-types/${encodeURIComponent(id)}`, patchBody),
  deleteTicketType: (id: string) => del<void>(`/api/ticket-types/${encodeURIComponent(id)}`),
};
