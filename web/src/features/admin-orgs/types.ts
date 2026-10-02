export type OrgInvitationStatus = "pending" | "accepted" | "revoked" | "expired";

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
