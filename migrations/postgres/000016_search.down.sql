DROP INDEX IF EXISTS idx_tickets_search_vec;
ALTER TABLE tickets DROP COLUMN IF EXISTS search_vec;
DROP INDEX IF EXISTS idx_users_search_vec;
ALTER TABLE users DROP COLUMN IF EXISTS search_vec;
