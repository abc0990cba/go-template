-- name: GetLoginGuard :one
SELECT failed_attempts, locked_until FROM login_guards WHERE user_id = $1;

-- name: RecordLoginFailure :one
INSERT INTO login_guards (user_id, failed_attempts, locked_until, updated_at)
VALUES ($1, 1, $2, now())
ON CONFLICT (user_id) DO UPDATE
SET failed_attempts = login_guards.failed_attempts + 1,
    locked_until = EXCLUDED.locked_until,
    updated_at = now()
RETURNING failed_attempts;

-- name: ResetLoginGuard :execrows
DELETE FROM login_guards WHERE user_id = $1;
