import { http } from "@/shared/api/core";
import { buildListQuery, type Paged } from "@/shared/api/pagination";
import type { ListParams } from "@/shared/api/pagination";

/** Endpoints of the admin-orgs slice: tenants, members, invitations. */
export function listOrgs(params: Partial<ListParams>) {
  return http.get<Paged<Organization>>(`/api/orgs?${buildListQuery(params)}`);
}

export function createOrg(name: string, slug: string) {
  return http.post<Organization>("/api/orgs", { name, slug });
}

export function getOrg(id: string) {
  return http.get<Organization>(`/api/orgs/${encodeURIComponent(id)}`);
}

export function updateOrg(id: string, name: string, slug: string) {
  return http.put<void>(`/api/orgs/${encodeURIComponent(id)}`, { name, slug });
}

export function deleteOrg(id: string) {
  return http.del<void>(`/api/orgs/${encodeURIComponent(id)}`);
}

export function orgMembers(id: string) {
  return http
    .get<Paged<OrgMember>>(`/api/orgs/${encodeURIComponent(id)}/members`)
    .then((p) => p.data);
}

export function addOrgMember(id: string, userId: string) {
  return http.post<void>(`/api/orgs/${encodeURIComponent(id)}/members`, { user_id: userId });
}

export function removeOrgMember(id: string, userId: string) {
  return http.del<void>(
    `/api/orgs/${encodeURIComponent(id)}/members/${encodeURIComponent(userId)}`,
  );
}

export function inviteOrgMember(orgId: string, email: string, role: string) {
  return http.post<OrgInvitation & { token: string }>(
    `/api/orgs/${encodeURIComponent(orgId)}/invitations`,
    {
      email,
      role,
    },
  );
}

export function orgInvitations(orgId: string) {
  return http
    .get<Paged<OrgInvitation>>(`/api/orgs/${encodeURIComponent(orgId)}/invitations`)
    .then((p) => p.data);
}

export function revokeOrgInvitation(orgId: string, invitationId: string) {
  return http.del<void>(
    `/api/orgs/${encodeURIComponent(orgId)}/invitations/${encodeURIComponent(invitationId)}`,
  );
}

export function myInvitations() {
  return http.get<Paged<OrgInvitation>>("/api/invitations").then((p) => p.data);
}

import type { Organization, OrgInvitation, OrgMember } from "./types";
