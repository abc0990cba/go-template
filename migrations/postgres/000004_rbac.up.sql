-- RBAC foundation: organizations (Casbin domains), memberships, role
-- metadata, and the policy table the Casbin adapter persists to.

CREATE TABLE organizations (
  id text PRIMARY KEY,
  name text NOT NULL,
  slug text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE org_members (
  org_id text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  joined_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (org_id, user_id)
);

CREATE INDEX idx_org_members_user ON org_members(user_id);

CREATE TABLE roles (
  key text PRIMARY KEY,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  is_system boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Casbin policy storage: one row per policy (ptype "p": v0 subject, v1
-- domain, v2 object, v3 action) or grouping rule (ptype "g": v0 user, v1
-- role, v2 domain).
CREATE TABLE casbin_rule (
  id bigserial PRIMARY KEY,
  ptype text NOT NULL,
  v0 text NOT NULL DEFAULT '',
  v1 text NOT NULL DEFAULT '',
  v2 text NOT NULL DEFAULT '',
  v3 text NOT NULL DEFAULT '',
  v4 text NOT NULL DEFAULT '',
  v5 text NOT NULL DEFAULT ''
);

CREATE INDEX idx_casbin_rule_lookup ON casbin_rule(ptype, v0, v1);

ALTER TABLE jobs ADD COLUMN org_id text NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN created_by text NOT NULL DEFAULT '';
ALTER TABLE reports ADD COLUMN org_id text NOT NULL DEFAULT '';
