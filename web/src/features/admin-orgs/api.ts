import { api, unwrap } from "@/shared/api/core";
import type { Paged, ListParams } from "@/shared/api/pagination";
import type { Organization, OrgInvitation, OrgMember } from "./types";

/** Endpoints of the admin-orgs slice: tenants, members, invitations. Every
 * call goes through the typed client, so paths, bodies, and responses are
 * checked against the OpenAPI contract at compile time. */

export function listOrgs(params: Partial<ListParams>): Promise<Paged<Organization>> {
  /** The spec vocabulary for orgs: the shared list params minus the
   * status/type/user_id filters orgs do not take. */
  return unwrap(
    api.GET("/api/orgs", {
      params: {
        query: {
          mode: params.mode,
          limit: params.limit,
          offset: params.offset,
          q: params.q,
        },
      },
    }),
  );
}

export function createOrg(name: string, slug: string): Promise<Organization> {
  return unwrap(api.POST("/api/orgs", { body: { name, slug } }));
}

export function getOrg(id: string): Promise<Organization> {
  return unwrap(api.GET("/api/orgs/{orgId}", { params: { path: { orgId: id } } }));
}

export function updateOrg(id: string, name: string, slug: string): Promise<void> {
  return unwrap(
    api.PUT("/api/orgs/{orgId}", { params: { path: { orgId: id } }, body: { name, slug } }),
  );
}

export function deleteOrg(id: string): Promise<void> {
  return unwrap(api.DELETE("/api/orgs/{orgId}", { params: { path: { orgId: id } } }));
}

export function orgMembers(id: string): Promise<OrgMember[]> {
  return unwrap(api.GET("/api/orgs/{orgId}/members", { params: { path: { orgId: id } } })).then(
    (p) => p.data,
  );
}

export function addOrgMember(id: string, userId: string): Promise<void> {
  return unwrap(
    api.POST("/api/orgs/{orgId}/members", {
      params: { path: { orgId: id } },
      body: { user_id: userId },
    }),
  );
}

export function removeOrgMember(id: string, userId: string): Promise<void> {
  return unwrap(
    api.DELETE("/api/orgs/{orgId}/members/{userId}", {
      params: { path: { orgId: id, userId } },
    }),
  );
}

export function inviteOrgMember(
  orgId: string,
  email: string,
  role: string,
): Promise<OrgInvitation> {
  return unwrap(
    api.POST("/api/orgs/{orgId}/invitations", {
      params: { path: { orgId } },
      body: { email, role },
    }),
  );
}

export function orgInvitations(orgId: string): Promise<OrgInvitation[]> {
  return unwrap(api.GET("/api/orgs/{orgId}/invitations", { params: { path: { orgId } } })).then(
    (p) => p.data,
  );
}

export function revokeOrgInvitation(orgId: string, invitationId: string): Promise<void> {
  return unwrap(
    api.DELETE("/api/orgs/{orgId}/invitations/{invId}", {
      params: { path: { orgId, invId: invitationId } },
    }),
  );
}

export function myInvitations(): Promise<OrgInvitation[]> {
  return unwrap(api.GET("/api/invitations")).then((p) => p.data);
}
