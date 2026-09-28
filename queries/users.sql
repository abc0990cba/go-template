-- name: CreateUser :one
INSERT INTO users (id, email, name, external_ref, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, email, name, external_ref, created_at, updated_at;

-- name: UpdateUser :one
UPDATE users
SET email = $2, name = $3, external_ref = $4, updated_at = now()
WHERE id = $1
RETURNING id, email, name, external_ref, created_at, updated_at;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = $1;

-- name: GetUser :one
SELECT id, email, name, external_ref, enrich_profile, enrich_contact, enriched_at, created_at, updated_at
FROM users
WHERE id = $1;

-- name: GetUserByExternalRef :one
SELECT id, email, name, external_ref, enrich_profile, enrich_contact, enriched_at, created_at, updated_at
FROM users
WHERE external_ref = $1;

-- name: GetUserByEmail :one
SELECT u.id, u.email, u.name, u.external_ref, u.enrich_profile, u.enrich_contact, u.enriched_at, u.created_at, u.updated_at,
       COALESCE(p.password_hash, '') AS password_hash
FROM users u
LEFT JOIN password_credentials p ON p.user_id = u.id
WHERE lower(u.email) = lower(sqlc.arg('email'));

-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: UpsertPasswordCredential :exec
INSERT INTO password_credentials (user_id, password_hash)
VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE
SET password_hash = EXCLUDED.password_hash, updated_at = now();

-- name: ListUsersOffset :many
SELECT u.id, u.email, u.name, u.external_ref, u.enrich_profile, u.enrich_contact, u.enriched_at, u.created_at, u.updated_at
FROM users u
WHERE (sqlc.arg('query')::text = '' OR (u.email || ' ' || u.name) ILIKE '%' || sqlc.arg('query')::text || '%')
ORDER BY u.created_at DESC, u.id ASC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- name: ListUsersKeyset :many
SELECT u.id, u.email, u.name, u.external_ref, u.enrich_profile, u.enrich_contact, u.enriched_at, u.created_at, u.updated_at
FROM users u
WHERE (sqlc.arg('query')::text = '' OR (u.email || ' ' || u.name) ILIKE '%' || sqlc.arg('query')::text || '%')
  AND (
    u.created_at < (SELECT c.created_at FROM users c WHERE c.id = sqlc.arg('after'))
    OR (u.created_at = (SELECT c.created_at FROM users c WHERE c.id = sqlc.arg('after')) AND u.id > sqlc.arg('after'))
  )
ORDER BY u.created_at DESC, u.id ASC
LIMIT sqlc.arg('lim');

-- name: DeleteDemoUsers :execrows
DELETE FROM users WHERE id LIKE 'demo-%';

-- name: DeletePasswordCredential :execrows
DELETE FROM password_credentials WHERE user_id = $1;

-- name: LockUserByExternalRefForSeq :one
SELECT id, enrich_profile_seq, enrich_contact_seq
FROM users
WHERE external_ref = $1
FOR UPDATE;
