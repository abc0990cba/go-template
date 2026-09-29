-- Organization invitations: single-use hashed tokens; the raw token lives
-- only in the invitation email.

-- name: CreateOrgInvitation :exec
INSERT INTO org_invitations (id, org_id, email, role, invited_by, token_hash, status, expires_at, created_at)
VALUES (sqlc.arg('id'), sqlc.arg('org_id'), sqlc.arg('email'), sqlc.arg('role'), sqlc.arg('invited_by'),
        sqlc.arg('token_hash'), 'pending', sqlc.arg('expires_at'), now());

-- name: GetOrgInvitationByHash :one
SELECT id, org_id, email, role, invited_by, token_hash, status, expires_at, accepted_at, accepted_by, created_at
FROM org_invitations
WHERE token_hash = sqlc.arg('token_hash');

-- name: GetOrgInvitation :one
SELECT id, org_id, email, role, invited_by, token_hash, status, expires_at, accepted_at, accepted_by, created_at
FROM org_invitations
WHERE id = sqlc.arg('id');

-- name: ListOrgInvitations :many
SELECT id, org_id, email, role, invited_by, token_hash, status, expires_at, accepted_at, accepted_by, created_at
FROM org_invitations
WHERE org_id = sqlc.arg('org_id')
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountOrgInvitations :one
SELECT count(*) FROM org_invitations WHERE org_id = sqlc.arg('org_id');

-- name: ListPendingOrgInvitationsByEmail :many
SELECT id, org_id, email, role, invited_by, token_hash, status, expires_at, accepted_at, accepted_by, created_at
FROM org_invitations
WHERE lower(email) = lower(sqlc.arg('email')) AND status = 'pending' AND expires_at > now()
ORDER BY created_at DESC;

-- name: SetOrgInvitationStatus :exec
UPDATE org_invitations
SET status = sqlc.arg('status'),
    accepted_at = COALESCE(sqlc.arg('accepted_at'), accepted_at),
    accepted_by = COALESCE(sqlc.arg('accepted_by'), accepted_by)
WHERE id = sqlc.arg('id');

-- name: ExpireOldOrgInvitations :exec
UPDATE org_invitations SET status = 'expired' WHERE status = 'pending' AND expires_at <= now();
