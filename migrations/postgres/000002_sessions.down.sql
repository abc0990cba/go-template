DROP TABLE IF EXISTS oauth_states;
DROP TABLE IF EXISTS sessions;
ALTER TABLE auth_identities
  DROP COLUMN name,
  DROP COLUMN avatar_url,
  DROP COLUMN last_login_at;
DROP INDEX IF EXISTS idx_users_email_lower;
CREATE INDEX idx_users_created_at_id ON users(created_at DESC, id DESC);
DROP INDEX IF EXISTS idx_users_created_at_id;
