-- Organization member invitations: a pending row per (org, email) with a
-- single-use hashed token. Accepting flips the status and creates the
-- membership + domain role grant.

CREATE TABLE org_invitations (
  id text PRIMARY KEY,
  org_id text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  email text NOT NULL,
  role text NOT NULL,
  invited_by text NOT NULL DEFAULT '',
  token_hash text NOT NULL UNIQUE,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'revoked', 'expired')),
  expires_at timestamptz NOT NULL,
  accepted_at timestamptz,
  accepted_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_org_invitations_org ON org_invitations(org_id, status, created_at DESC);
CREATE INDEX idx_org_invitations_email ON org_invitations(email, status);
