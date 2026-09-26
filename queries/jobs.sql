-- name: UpsertJob :exec
INSERT INTO jobs (id, type, status, payload, error, created_at, updated_at)
VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7)
ON CONFLICT (id) DO UPDATE
SET type = EXCLUDED.type,
    status = EXCLUDED.status,
    payload = EXCLUDED.payload,
    error = EXCLUDED.error,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at;

-- name: GetJob :one
SELECT id, type, status, payload, error, created_at, updated_at
FROM jobs
WHERE id = $1;

-- name: ListJobs :many
SELECT id, type, status, payload, error, created_at, updated_at
FROM jobs
ORDER BY created_at DESC;
