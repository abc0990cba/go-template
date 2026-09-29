-- Full-text search columns: generated tsvector over the human-readable
-- fields, GIN-indexed. The 'simple' configuration avoids language-specific
-- stemming so mixed EN/RU content stays searchable.

ALTER TABLE users ADD COLUMN search_vec tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', coalesce(name, '') || ' ' || coalesce(email, ''))) STORED;
CREATE INDEX idx_users_search_vec ON users USING GIN (search_vec);

ALTER TABLE tickets ADD COLUMN search_vec tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(description, ''))) STORED;
CREATE INDEX idx_tickets_search_vec ON tickets USING GIN (search_vec);
