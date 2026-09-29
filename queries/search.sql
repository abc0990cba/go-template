-- Full-text search over the two primary entities. Ranked by ts_rank with a
-- recency tiebreak; limit is clamped by the service.

-- name: SearchUsersByVector :many
SELECT id, email, name, created_at, updated_at FROM users
WHERE search_vec @@ websearch_to_tsquery('simple', sqlc.arg('query'))
ORDER BY ts_rank(search_vec, websearch_to_tsquery('simple', sqlc.arg('query'))) DESC, created_at DESC
LIMIT sqlc.arg('limit');

-- name: SearchTicketsByVector :many
SELECT id, number, org_id, type_key, title, status, priority, created_by, created_at, updated_at FROM tickets
WHERE org_id = sqlc.arg('org_id')
  AND search_vec @@ websearch_to_tsquery('simple', sqlc.arg('query'))
ORDER BY ts_rank(search_vec, websearch_to_tsquery('simple', sqlc.arg('query'))) DESC, created_at DESC
LIMIT sqlc.arg('limit');
