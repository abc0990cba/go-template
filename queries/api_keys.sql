-- name: InsertAPIKey :exec
INSERT INTO api_keys (id, user_id, name, prefix, key_hash, scopes, org_id, expires_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListAPIKeysForUser :many
SELECT id, user_id, name, prefix, scopes, org_id, last_used_at, expires_at, revoked_at, created_at
FROM api_keys
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: GetAPIKeyByPrefix :one
SELECT id, user_id, name, prefix, key_hash, scopes, org_id, last_used_at, expires_at, revoked_at, created_at
FROM api_keys
WHERE prefix = $1 AND revoked_at IS NULL;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = now() WHERE id = $1;

-- name: RevokeAPIKey :execrows
UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL;

-- name: DeleteDemoAPIKeys :execrows
DELETE FROM api_keys WHERE id LIKE 'demo-%' OR user_id LIKE 'demo-%';
