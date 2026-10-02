import { http } from "@/shared/api/core";
import type { Paged } from "@/shared/api/pagination";

/** Endpoints of the admin-rbac slice: roles, their permission sets, and
 * the raw Casbin policy rules. */
export function listRoles() {
  return http.get<Paged<Role>>("/api/roles").then((p) => p.data);
}

export function createRole(key: string, name: string, description: string) {
  return http.post<Role>("/api/roles", { key, name, description });
}

export function updateRole(key: string, name: string, description: string) {
  return http.put<void>(`/api/roles/${encodeURIComponent(key)}`, { name, description });
}

export function deleteRole(key: string) {
  return http.del<void>(`/api/roles/${encodeURIComponent(key)}`);
}

export function rolePermissions(key: string) {
  return http
    .get<Paged<PolicyRule>>(`/api/roles/${encodeURIComponent(key)}/permissions`)
    .then((p) => p.data);
}

export function setRolePermissions(key: string, domain: string, permissions: Permission[]) {
  return http.put<void>(`/api/roles/${encodeURIComponent(key)}/permissions`, {
    domain,
    permissions,
  });
}

export function listPolicies(ptype: "p" | "g") {
  return http.get<Paged<PolicyRule>>(`/api/policies?ptype=${ptype}`).then((p) => p.data);
}

export function addPolicy(rule: PolicyRule) {
  return http.post<void>("/api/policies", rule);
}

export function removePolicy(rule: PolicyRule) {
  return http.del<void>("/api/policies", rule);
}

export function rbacCatalog() {
  return http.get<{ resources: RbacResource[] }>("/api/rbac/catalog").then((r) => r.resources);
}

import type { Permission, PolicyRule, RbacResource, Role } from "./types";
