-- name: CreateUser :one
INSERT INTO users (id, email, name)
VALUES ($1, $2, $3)
RETURNING id, email, name, created_at, updated_at;

-- name: GetUser :one
SELECT id, email, name, created_at, updated_at
FROM users
WHERE id = $1;

-- name: ListUsersKeyset :many
SELECT id, email, name, created_at, updated_at
FROM users
WHERE (created_at, id) < ($1, $2)
ORDER BY created_at DESC, id DESC
LIMIT $3;
