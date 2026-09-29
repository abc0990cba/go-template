-- Refresh tokens for the JWT grant: rotation with reuse detection. Only
-- sha256 hashes are stored; the raw token exists only in the API response.

-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (id, user_id, family_id, token_hash, expires_at, user_agent, ip, created_at)
VALUES (sqlc.arg('id'), sqlc.arg('user_id'), sqlc.arg('family_id'), sqlc.arg('token_hash'), sqlc.arg('expires_at'),
        sqlc.arg('user_agent'), sqlc.arg('ip'), now());

-- name: GetRefreshTokenByHash :one
SELECT id, user_id, family_id, token_hash, expires_at, revoked_at, replaced_by, user_agent, ip, created_at
FROM refresh_tokens
WHERE token_hash = sqlc.arg('token_hash');

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked_at = now() WHERE id = sqlc.arg('id') AND revoked_at IS NULL;

-- name: RevokeRefreshFamily :exec
UPDATE refresh_tokens SET revoked_at = now()
WHERE family_id = sqlc.arg('family_id') AND revoked_at IS NULL;

-- name: RevokeRefreshTokensForUser :exec
UPDATE refresh_tokens SET revoked_at = now()
WHERE user_id = sqlc.arg('user_id') AND revoked_at IS NULL;

-- name: DeleteExpiredRefreshTokens :exec
DELETE FROM refresh_tokens WHERE expires_at < now() - interval '7 days';
