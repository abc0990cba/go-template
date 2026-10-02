/** Entities every layer talks about: the account, the session probe. */

export interface User {
  id: string;
  email: string;
  name: string;
  external_ref?: string;
  enrich_profile?: string;
  enrich_contact?: string;
  enriched_at?: string | null;
  created_at: string;
  updated_at: string;
}

/** GET /api/me: the account plus the RBAC context the UI gates on. */
export interface Me extends User {
  orgs: { id: string; name: string; slug: string; created_at: string; updated_at: string }[];
  active_org: string;
  roles: string[];
  permissions: string[];
  impersonated: boolean;
  impersonator_id?: string;
  impersonator_name?: string;
}

export interface SessionInfo {
  id: string;
  created_at: string;
  expires_at: string;
  current: boolean;
}
