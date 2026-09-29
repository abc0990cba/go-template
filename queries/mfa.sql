-- Multi-factor authentication: TOTP factor, recovery codes, login
-- challenges. Challenge tokens and recovery codes are stored hashed.

-- name: UpsertMFAFactor :exec
INSERT INTO mfa_factors (user_id, secret, confirmed, created_at, updated_at)
VALUES (sqlc.arg('user_id'), sqlc.arg('secret'), false, now(), now())
ON CONFLICT (user_id) DO UPDATE
SET secret = EXCLUDED.secret, confirmed = false, confirmed_at = NULL, updated_at = now();

-- name: ConfirmMFAFactor :exec
UPDATE mfa_factors
SET confirmed = true, confirmed_at = now(), updated_at = now()
WHERE user_id = sqlc.arg('user_id');

-- name: GetMFAFactor :one
SELECT user_id, secret, confirmed, created_at, confirmed_at, updated_at
FROM mfa_factors
WHERE user_id = sqlc.arg('user_id');

-- name: DeleteMFAFactor :exec
DELETE FROM mfa_factors WHERE user_id = sqlc.arg('user_id');

-- name: CreateMFARecoveryCode :exec
INSERT INTO mfa_recovery_codes (id, user_id, code_hash, created_at)
VALUES (sqlc.arg('id'), sqlc.arg('user_id'), sqlc.arg('code_hash'), now());

-- name: ListMFARecoveryCodes :many
SELECT id, user_id, code_hash, used_at, created_at
FROM mfa_recovery_codes
WHERE user_id = sqlc.arg('user_id');

-- name: GetUnusedMFARecoveryCodeByHash :one
SELECT id, user_id, code_hash, used_at, created_at
FROM mfa_recovery_codes
WHERE user_id = sqlc.arg('user_id') AND code_hash = sqlc.arg('code_hash') AND used_at IS NULL;

-- name: MarkMFARecoveryCodeUsed :exec
UPDATE mfa_recovery_codes SET used_at = now() WHERE id = sqlc.arg('id');

-- name: DeleteMFARecoveryCodes :exec
DELETE FROM mfa_recovery_codes WHERE user_id = sqlc.arg('user_id');

-- name: CreateMFAChallenge :exec
INSERT INTO mfa_challenges (id, user_id, token_hash, attempts, expires_at, created_at)
VALUES (sqlc.arg('id'), sqlc.arg('user_id'), sqlc.arg('token_hash'), 0, sqlc.arg('expires_at'), now());

-- name: GetMFAChallengeByHash :one
SELECT id, user_id, token_hash, attempts, expires_at, created_at
FROM mfa_challenges
WHERE token_hash = sqlc.arg('token_hash');

-- name: IncrementMFAChallengeAttempts :exec
UPDATE mfa_challenges SET attempts = attempts + 1 WHERE id = sqlc.arg('id');

-- name: DeleteMFAChallenge :exec
DELETE FROM mfa_challenges WHERE id = sqlc.arg('id');

-- name: DeleteExpiredMFAChallenges :exec
DELETE FROM mfa_challenges WHERE expires_at < now();

-- name: CountMFAFactors :one
SELECT count(*) FROM mfa_factors WHERE confirmed = true;
