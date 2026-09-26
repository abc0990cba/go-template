DROP INDEX IF EXISTS idx_enrichments_user_created;
DROP INDEX IF EXISTS idx_reports_created_id;
DROP INDEX IF EXISTS idx_jobs_status;
DROP INDEX IF EXISTS idx_jobs_created_id;
DROP INDEX IF EXISTS idx_enrichments_seq;
ALTER TABLE user_enrichment_profiles DROP COLUMN IF EXISTS seq;
