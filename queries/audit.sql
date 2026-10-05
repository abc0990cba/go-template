-- name: InsertAudit :exec
INSERT INTO audit_log (actor_id, actor_email, action, resource_type, resource_id, domain, diff, ip, request_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10);

-- name: ListAuditOffset :many
SELECT id, actor_id, actor_email, action, resource_type, resource_id, domain, diff, ip, request_id, created_at
FROM audit_log
WHERE (sqlc.arg('actor')::text = '' OR actor_id = sqlc.arg('actor')::text)
  AND (sqlc.arg('action')::text = '' OR action = sqlc.arg('action')::text)
  AND (sqlc.arg('resource')::text = '' OR resource_type = sqlc.arg('resource')::text)
  AND (sqlc.arg('resource_id')::text = '' OR resource_id = sqlc.arg('resource_id')::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- name: ListAuditSubject :many
-- One user's history: everything they did (as actor) plus everything that
-- happened to their account (resource_type 'user' entries pointing at it).
SELECT id, actor_id, actor_email, action, resource_type, resource_id, domain, diff, ip, request_id, created_at
FROM audit_log
WHERE actor_id = sqlc.arg('subject')::text
   OR (resource_type = 'user' AND resource_id = sqlc.arg('subject')::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- name: PruneAuditLog :one
-- Retention runs through the SECURITY DEFINER function (migration 000022):
-- direct DELETE on audit_log is trigger-blocked for the application role.
SELECT audit_prune(sqlc.arg('cutoff')::timestamptz) AS pruned;

-- name: DeleteDemoAudit :execrows
DELETE FROM audit_log WHERE actor_id LIKE 'demo-%';
