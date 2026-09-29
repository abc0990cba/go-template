-- Refresh-token families for the JWT password grant. Rotation on every use;
-- presenting a rotated (revoked) token is a breach signal and kills the
-- whole family. Only sha256 hashes are stored.

CREATE TABLE refresh_tokens (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  family_id text NOT NULL,
  token_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  replaced_by text NOT NULL DEFAULT '',
  user_agent text NOT NULL DEFAULT '',
  ip text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id, created_at DESC);
CREATE INDEX idx_refresh_tokens_family ON refresh_tokens(family_id);
