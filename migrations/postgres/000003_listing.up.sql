-- Listing support: keyset pagination for jobs/reports/enrichments plus the
-- filters the SPA tables expose.

-- Enrichment rows have no id; a sequence gives keyset pagination a stable
-- unique tiebreaker. Existing rows are backfilled automatically.
ALTER TABLE user_enrichment_profiles ADD COLUMN seq bigserial;
CREATE UNIQUE INDEX idx_enrichments_seq ON user_enrichment_profiles (seq);

-- Sort and filter paths of the new list queries.
CREATE INDEX idx_jobs_created_id ON jobs (created_at DESC, id ASC);
CREATE INDEX idx_jobs_status ON jobs (status, created_at DESC);
CREATE INDEX idx_reports_created_id ON reports (created_at DESC, id ASC);
CREATE INDEX idx_enrichments_user_created ON user_enrichment_profiles (user_id, created_at DESC);
