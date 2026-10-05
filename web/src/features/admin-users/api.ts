import { api, unwrap, unwrapWithETag } from "@/shared/api/core";
import type { Paged, ListParams } from "@/shared/api/pagination";
import type { operations } from "@/shared/api/schema.gen";
import type { User } from "@/shared/types/core";
import type { AuditEntry } from "@/features/admin-insights";
import type { PolicyRule } from "@/features/admin-rbac";

/** Endpoints of the admin-users slice. Every call goes through the typed
 * client, so paths, bodies, and responses are checked against the OpenAPI
 * contract at compile time. */

type ListUsersQuery = operations["listUsers"]["parameters"]["query"];

/** The spec types the impersonation session fields as optional. */
type ImpersonationSession =
  operations["startImpersonation"]["responses"][200]["content"]["application/json"];

/** The shared list vocabulary. The URL keeps the extra filters as free
 * strings; the backend ignores the ones the user list does not scope on. */
export function listUsers(params: Partial<ListParams>): Promise<Paged<User>> {
  return unwrap(api.GET("/api/users", { params: { query: params as ListUsersQuery } }));
}

export function createUser(email: string, name: string, external_ref?: string): Promise<User> {
  return unwrap(api.POST("/api/users", { body: { email, name, external_ref } }));
}

export function getUser(id: string): Promise<User> {
  return unwrap(api.GET("/api/users/{id}", { params: { path: { id } } }));
}

/** getUserWithETag pairs the record with its ETag for optimistic writes. */
export function getUserWithETag(id: string): Promise<{ data: User; etag: string }> {
  return unwrapWithETag(api.GET("/api/users/{id}", { params: { path: { id } } }));
}

export function updateUser(
  id: string,
  email: string,
  name: string,
  external_ref?: string,
  etag?: string,
): Promise<User> {
  return unwrap(
    api.PUT("/api/users/{id}", {
      params: { path: { id }, header: etag ? { "If-Match": etag } : undefined },
      body: { email, name, external_ref },
    }),
  );
}

export function deleteUser(id: string): Promise<void> {
  return unwrap(api.DELETE("/api/users/{id}", { params: { path: { id } } }));
}

export function resetUserMFA(id: string): Promise<void> {
  return unwrap(api.DELETE("/api/users/{id}/mfa", { params: { path: { id } } }));
}

export function impersonate(userId: string): Promise<ImpersonationSession> {
  return unwrap(api.POST("/api/impersonation", { body: { user_id: userId } }));
}

export function stopImpersonation(): Promise<void> {
  return unwrap(api.POST("/api/impersonation/stop"));
}

export function userActivity(id: string, limit = 25, offset = 0): Promise<Paged<AuditEntry>> {
  return unwrap(
    api.GET("/api/users/{id}/activity", { params: { path: { id }, query: { limit, offset } } }),
  );
}

export function userRoles(id: string): Promise<PolicyRule[]> {
  return unwrap(api.GET("/api/users/{id}/roles", { params: { path: { id } } })).then((p) => p.data);
}

export function assignUserRole(id: string, role: string, domain: string): Promise<void> {
  return unwrap(
    api.POST("/api/users/{id}/roles", { params: { path: { id } }, body: { role, domain } }),
  );
}

export function unassignUserRole(id: string, role: string, domain: string): Promise<void> {
  return unwrap(
    api.DELETE("/api/users/{id}/roles", { params: { path: { id } }, body: { role, domain } }),
  );
}

export function triggerEnrichDemo(id: string, source: "kafka" | "nats") {
  return unwrap(
    api.POST("/api/users/{id}/enrich-demo", { params: { path: { id } }, body: { source } }),
  );
}
