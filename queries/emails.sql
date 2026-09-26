-- name: CreateEmailToken :exec
INSERT INTO email_tokens (id, user_id, kind, token_hash, expires_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetEmailTokenByHash :one
SELECT id, user_id, kind, token_hash, expires_at, used_at, created_at
FROM email_tokens
WHERE token_hash = $1;

-- name: MarkEmailTokenUsed :execrows
UPDATE email_tokens SET used_at = now() WHERE id = $1 AND used_at IS NULL;

-- name: DeleteDemoEmailTokens :execrows
DELETE FROM email_tokens WHERE id LIKE 'demo-%' OR user_id LIKE 'demo-%';
