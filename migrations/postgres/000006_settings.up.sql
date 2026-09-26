-- System settings and feature flags: a typed key-value store the admin UI
-- edits at runtime; values are JSON documents (booleans, numbers, strings).

CREATE TABLE settings (
  key text PRIMARY KEY,
  value jsonb NOT NULL DEFAULT 'null',
  description text NOT NULL DEFAULT '',
  updated_by text NOT NULL DEFAULT '',
  updated_at timestamptz NOT NULL DEFAULT now()
);
