-- name: CreateSession :exec
INSERT INTO sessions (token, user_id, expires_at)
VALUES ($1, $2, now() + make_interval(secs => $3));

-- name: GetSession :one
UPDATE sessions
SET expires_at = now() + make_interval(secs => $2)
WHERE token = $1 AND expires_at > now()
RETURNING user_id;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = $1;

-- name: DeleteSessionsForUser :exec
DELETE FROM sessions WHERE user_id = $1;
