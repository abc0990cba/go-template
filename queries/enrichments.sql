-- name: AddEnrichment :exec
INSERT INTO user_enrichment_profiles (user_id, source, payload, created_at)
VALUES ($1, $2, $3::jsonb, $4);

-- name: CountEnrichments :one
SELECT count(*) FROM user_enrichment_profiles;

-- name: ListEnrichmentsOffset :many
SELECT user_id, source, payload, created_at, seq
FROM user_enrichment_profiles
WHERE (sqlc.arg('query')::text = '' OR (source || ' ' || payload) ILIKE '%' || sqlc.arg('query')::text || '%')
  AND (sqlc.arg('user_id')::text = '' OR user_id = sqlc.arg('user_id')::text)
ORDER BY created_at DESC, seq ASC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- name: ListEnrichmentsKeyset :many
SELECT user_id, source, payload, created_at, seq
FROM user_enrichment_profiles
WHERE (sqlc.arg('query')::text = '' OR (source || ' ' || payload) ILIKE '%' || sqlc.arg('query')::text || '%')
  AND (sqlc.arg('user_id')::text = '' OR user_id = sqlc.arg('user_id')::text)
  AND (
    created_at < sqlc.arg('after_at')::timestamptz
    OR (created_at = sqlc.arg('after_at')::timestamptz AND seq > sqlc.arg('after_seq'))
  )
ORDER BY created_at DESC, seq ASC
LIMIT sqlc.arg('lim');

-- name: DeleteDemoEnrichments :execrows
DELETE FROM user_enrichment_profiles WHERE user_id LIKE 'demo-%' OR user_id = '';
