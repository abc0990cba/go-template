-- name: SaveOAuthState :exec
INSERT INTO oauth_states (state, provider, user_id, redirect_after, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ConsumeOAuthState :one
DELETE FROM oauth_states
WHERE state = $1
RETURNING state, provider, user_id, redirect_after, expires_at;
