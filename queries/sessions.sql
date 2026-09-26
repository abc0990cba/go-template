-- name: CreateSession :exec
INSERT INTO sessions (token, user_id, impersonated_by, expires_at)
VALUES ($1, $2, $3, now() + make_interval(secs => $4));

-- name: GetSession :one
UPDATE sessions
SET expires_at = now() + make_interval(secs => $2)
WHERE token = $1 AND expires_at > now()
RETURNING user_id, impersonated_by;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = $1;

-- name: DeleteSessionsForUser :exec
DELETE FROM sessions WHERE user_id = $1;

-- name: ListSessions :many
SELECT left(token, 8) AS id, created_at, expires_at
FROM sessions
WHERE user_id = $1 AND expires_at > now()
ORDER BY created_at DESC;
