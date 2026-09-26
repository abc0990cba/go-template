-- Sessions, OAuth state, and schema alignment with the domain model.

ALTER TABLE auth_identities
  ADD COLUMN name text NOT NULL DEFAULT '',
  ADD COLUMN avatar_url text NOT NULL DEFAULT '',
  ADD COLUMN last_login_at timestamptz NOT NULL DEFAULT now();

-- Emails are unique case-insensitively, matching the domain contract.
CREATE UNIQUE INDEX idx_users_email_lower ON users(lower(email));

-- Keyset pagination walks (created_at DESC, id ASC); rebuild the index to
-- match that order.
DROP INDEX idx_users_created_at_id;
CREATE INDEX idx_users_created_at_id ON users(created_at DESC, id ASC);

CREATE TABLE sessions (
  token text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL DEFAULT now() + interval '7 days',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);

CREATE TABLE oauth_states (
  state text PRIMARY KEY,
  provider text NOT NULL,
  user_id text NOT NULL DEFAULT '',
  redirect_after text NOT NULL DEFAULT '',
  expires_at timestamptz NOT NULL
);
