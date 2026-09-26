-- name: UpsertReport :exec
INSERT INTO reports (id, status, result, error, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO UPDATE
SET status = EXCLUDED.status,
    result = EXCLUDED.result,
    error = EXCLUDED.error,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at;

-- name: ListReports :many
SELECT id, status, result, error, created_at, updated_at
FROM reports
ORDER BY created_at DESC;
