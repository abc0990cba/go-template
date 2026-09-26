-- Single-use email tokens: password resets and address verification. Only
-- the sha256 hash of the token is stored.

CREATE TABLE email_tokens (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('reset', 'verify')),
  token_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_email_tokens_user ON email_tokens(user_id, kind);
