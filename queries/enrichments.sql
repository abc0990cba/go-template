-- name: AddEnrichment :exec
INSERT INTO user_enrichment_profiles (user_id, source, payload, created_at)
VALUES ($1, $2, $3::jsonb, $4);

-- name: CountEnrichments :one
SELECT count(*) FROM user_enrichment_profiles;
