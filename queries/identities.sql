-- name: InsertIdentity :exec
INSERT INTO auth_identities (provider, provider_subject, user_id, email, name, avatar_url, linked_at, last_login_at)
VALUES ($1, $2, $3, $4, $5, $6, now(), now());

-- name: GetIdentity :one
SELECT provider, provider_subject, user_id, email, name, avatar_url, linked_at, last_login_at
FROM auth_identities
WHERE provider = $1 AND provider_subject = $2;

-- name: GetIdentityOwner :one
SELECT user_id FROM auth_identities
WHERE provider = $1 AND provider_subject = $2;

-- name: UnlinkIdentity :exec
DELETE FROM auth_identities
WHERE user_id = $1 AND provider = $2;

-- name: ListIdentities :many
SELECT provider, provider_subject, user_id, email, name, avatar_url, linked_at, last_login_at
FROM auth_identities
WHERE user_id = $1
ORDER BY linked_at ASC;
