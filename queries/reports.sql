-- name: UpsertReport :exec
INSERT INTO reports (id, status, result, error, org_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE
SET status = EXCLUDED.status,
    result = EXCLUDED.result,
    error = EXCLUDED.error,
    org_id = EXCLUDED.org_id,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at;

-- name: ListReportsOffset :many
SELECT id, status, result, error, org_id, created_at, updated_at
FROM reports
WHERE (sqlc.arg('query')::text = '' OR (status || ' ' || result) ILIKE '%' || sqlc.arg('query')::text || '%')
ORDER BY created_at DESC, id ASC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- name: ListReportsKeyset :many
SELECT id, status, result, error, org_id, created_at, updated_at
FROM reports
WHERE (sqlc.arg('query')::text = '' OR (status || ' ' || result) ILIKE '%' || sqlc.arg('query')::text || '%')
  AND (
    created_at < (SELECT c.created_at FROM reports c WHERE c.id = sqlc.arg('after'))
    OR (created_at = (SELECT c.created_at FROM reports c WHERE c.id = sqlc.arg('after')) AND id > sqlc.arg('after'))
  )
ORDER BY created_at DESC, id ASC
LIMIT sqlc.arg('lim');

-- name: GetReport :one
SELECT id, status, result, error, org_id, created_at, updated_at
FROM reports
WHERE id = $1;

-- name: DeleteDemoReports :execrows
DELETE FROM reports WHERE id LIKE 'demo-%';
