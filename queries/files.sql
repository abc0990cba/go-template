-- File metadata. Blobs live in the file store; this table carries the
-- ownership and attachment relationships used for permission checks.

-- name: InsertFile :exec
INSERT INTO files (id, owner_id, org_id, key, filename, content_type, size_bytes, checksum_sha256, ref_type, ref_id, created_at)
VALUES (sqlc.arg('id'), sqlc.arg('owner_id'), sqlc.arg('org_id'), sqlc.arg('key'), sqlc.arg('filename'),
        sqlc.arg('content_type'), sqlc.arg('size_bytes'), sqlc.arg('checksum_sha256'),
        sqlc.arg('ref_type'), sqlc.arg('ref_id'), now());

-- name: GetFile :one
SELECT id, owner_id, org_id, key, filename, content_type, size_bytes, checksum_sha256, ref_type, ref_id, created_at, deleted_at
FROM files
WHERE id = sqlc.arg('id') AND deleted_at IS NULL;

-- name: ListFilesByRef :many
SELECT id, owner_id, org_id, key, filename, content_type, size_bytes, checksum_sha256, ref_type, ref_id, created_at, deleted_at
FROM files
WHERE ref_type = sqlc.arg('ref_type') AND ref_id = sqlc.arg('ref_id') AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: ListFilesByOwner :many
SELECT id, owner_id, org_id, key, filename, content_type, size_bytes, checksum_sha256, ref_type, ref_id, created_at, deleted_at
FROM files
WHERE owner_id = sqlc.arg('owner_id') AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: SoftDeleteFile :exec
UPDATE files SET deleted_at = now() WHERE id = sqlc.arg('id') AND deleted_at IS NULL;

-- name: SumFileBytesForUser :one
SELECT COALESCE(sum(size_bytes), 0)::bigint FROM files WHERE owner_id = sqlc.arg('owner_id') AND deleted_at IS NULL;

-- name: ListExportFilesOlderThan :many
SELECT id, owner_id, org_id, key, filename, content_type, size_bytes, checksum_sha256, ref_type, ref_id, created_at, deleted_at
FROM files
WHERE ref_type = 'export' AND deleted_at IS NULL AND created_at < sqlc.arg('older_than');
