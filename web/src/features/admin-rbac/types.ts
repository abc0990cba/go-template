export interface Role {
  key: string;
  name: string;
  description: string;
  is_system: boolean;
  created_at: string;
}

/** ptype "p": subject/domain/object/action; ptype "g": user/role/domain. */
export interface PolicyRule {
  ptype: "p" | "g";
  subject: string;
  domain: string;
  object?: string;
  action?: string;
}

export interface RbacResource {
  key: string;
  description: string;
  actions: string[];
}

export interface Permission {
  object: string;
  action: string;
}
