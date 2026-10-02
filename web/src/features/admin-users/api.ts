import { http } from "@/shared/api/core";
import { buildListQuery, type Paged } from "@/shared/api/pagination";
import type { ListParams } from "@/shared/api/pagination";

/** Endpoints of the admin-users slice. */
export function listUsers(params: Partial<ListParams>) {
  return http.get<Paged<User>>(`/api/users?${buildListQuery(params)}`);
}

export function createUser(email: string, name: string, external_ref?: string) {
  return http.post<User>("/api/users", { email, name, external_ref });
}

export function getUser(id: string) {
  return http.get<User>(`/api/users/${encodeURIComponent(id)}`);
}

export function updateUser(id: string, email: string, name: string, external_ref?: string) {
  return http.put<User>(`/api/users/${encodeURIComponent(id)}`, { email, name, external_ref });
}

export function deleteUser(id: string) {
  return http.del<void>(`/api/users/${encodeURIComponent(id)}`);
}

export function resetUserMFA(id: string) {
  return http.del<void>(`/api/users/${encodeURIComponent(id)}/mfa`);
}

export function impersonate(userId: string) {
  return http.post<{ status: string; user_id: string }>("/api/impersonation", { user_id: userId });
}

export function stopImpersonation() {
  return http.post<void>("/api/impersonation/stop");
}

export function userActivity(id: string, limit = 25, offset = 0) {
  return http.get<Paged<AuditEntry>>(
    `/api/users/${encodeURIComponent(id)}/activity?limit=${limit}&offset=${offset}`,
  );
}

export function userRoles(id: string) {
  return http
    .get<Paged<PolicyRule>>(`/api/users/${encodeURIComponent(id)}/roles`)
    .then((p) => p.data);
}

export function assignUserRole(id: string, role: string, domain: string) {
  return http.post<void>(`/api/users/${encodeURIComponent(id)}/roles`, { role, domain });
}

export function unassignUserRole(id: string, role: string, domain: string) {
  return http.del<void>(`/api/users/${encodeURIComponent(id)}/roles`, { role, domain });
}

export function triggerEnrichDemo(id: string, source: "kafka" | "nats") {
  return http.post<{ published: boolean; source: string; event_id: string; seq: number }>(
    `/api/users/${encodeURIComponent(id)}/enrich-demo`,
    { source },
  );
}

import type { User } from "@/shared/types/core";
import type { AuditEntry } from "@/features/admin-insights";
import type { PolicyRule } from "@/features/admin-rbac";
