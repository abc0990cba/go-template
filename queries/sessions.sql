-- name: CreateSession :exec
INSERT INTO sessions (token, user_id)
VALUES ($1, $2);

-- name: GetSession :one
SELECT user_id FROM sessions
WHERE token = $1 AND expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = $1;
