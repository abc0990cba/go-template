-- name: UpsertJob :exec
INSERT INTO jobs (id, type, status, payload, error, org_id, created_by, created_at, updated_at)
VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE
SET type = EXCLUDED.type,
    status = EXCLUDED.status,
    payload = EXCLUDED.payload,
    error = EXCLUDED.error,
    org_id = EXCLUDED.org_id,
    created_by = EXCLUDED.created_by,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at;

-- name: GetJob :one
SELECT id, type, status, payload, error, org_id, created_by, created_at, updated_at
FROM jobs
WHERE id = $1;

-- name: ListJobsOffset :many
SELECT id, type, status, payload, error, org_id, created_by, created_at, updated_at
FROM jobs
WHERE (sqlc.arg('query')::text = '' OR (type || ' ' || error) ILIKE '%' || sqlc.arg('query')::text || '%')
  AND (sqlc.arg('status')::text = '' OR status = sqlc.arg('status')::text)
  AND (sqlc.arg('typ')::text = '' OR type = sqlc.arg('typ')::text)
ORDER BY created_at DESC, id ASC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- name: ListJobsKeyset :many
SELECT id, type, status, payload, error, org_id, created_by, created_at, updated_at
FROM jobs
WHERE (sqlc.arg('query')::text = '' OR (type || ' ' || error) ILIKE '%' || sqlc.arg('query')::text || '%')
  AND (sqlc.arg('status')::text = '' OR status = sqlc.arg('status')::text)
  AND (sqlc.arg('typ')::text = '' OR type = sqlc.arg('typ')::text)
  AND (
    created_at < (SELECT c.created_at FROM jobs c WHERE c.id = sqlc.arg('after'))
    OR (created_at = (SELECT c.created_at FROM jobs c WHERE c.id = sqlc.arg('after')) AND id > sqlc.arg('after'))
  )
ORDER BY created_at DESC, id ASC
LIMIT sqlc.arg('lim');

-- name: DeleteDemoJobs :execrows
DELETE FROM jobs WHERE id LIKE 'demo-%';
