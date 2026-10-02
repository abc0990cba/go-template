import { api, unwrap } from "@/shared/api/core";
import type { Permission, PolicyRule, RbacResource, Role } from "./types";

/** Endpoints of the admin-rbac slice: roles, their permission sets, and
 * the raw Casbin policy rules. Every call goes through the typed client,
 * so paths, bodies, and responses are checked against the OpenAPI contract
 * at compile time. */

export function listRoles(): Promise<Role[]> {
  return unwrap(api.GET("/api/roles")).then((p) => p.data);
}

export function createRole(key: string, name: string, description: string): Promise<Role> {
  return unwrap(api.POST("/api/roles", { body: { key, name, description } }));
}

export function updateRole(key: string, name: string, description: string): Promise<void> {
  return unwrap(
    api.PUT("/api/roles/{key}", { params: { path: { key } }, body: { name, description } }),
  );
}

export function deleteRole(key: string): Promise<void> {
  return unwrap(api.DELETE("/api/roles/{key}", { params: { path: { key } } }));
}

export function rolePermissions(key: string): Promise<PolicyRule[]> {
  return unwrap(api.GET("/api/roles/{key}/permissions", { params: { path: { key } } })).then(
    (p) => p.data,
  );
}

export function setRolePermissions(
  key: string,
  domain: string,
  permissions: Permission[],
): Promise<void> {
  return unwrap(
    api.PUT("/api/roles/{key}/permissions", {
      params: { path: { key } },
      body: { domain, permissions },
    }),
  );
}

export function listPolicies(ptype: "p" | "g"): Promise<PolicyRule[]> {
  return unwrap(api.GET("/api/policies", { params: { query: { ptype } } })).then((p) => p.data);
}

export function addPolicy(rule: PolicyRule): Promise<void> {
  return unwrap(api.POST("/api/policies", { body: rule }));
}

export function removePolicy(rule: PolicyRule): Promise<void> {
  return unwrap(api.DELETE("/api/policies", { body: rule }));
}

export function rbacCatalog(): Promise<RbacResource[]> {
  return unwrap(api.GET("/api/rbac/catalog")).then((r) => r.resources);
}
