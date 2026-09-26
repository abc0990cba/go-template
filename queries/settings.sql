-- name: UpsertSetting :exec
INSERT INTO settings (key, value, description, updated_by, updated_at)
VALUES ($1, $2::jsonb, $3, $4, $5)
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value, description = EXCLUDED.description,
    updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at;

-- name: GetSetting :one
SELECT key, value, description, updated_by, updated_at
FROM settings
WHERE key = $1;

-- name: ListSettings :many
SELECT key, value, description, updated_by, updated_at
FROM settings
ORDER BY key ASC;
