-- name: InsertAudit :exec
INSERT INTO audit_log (actor_id, actor_email, action, resource_type, resource_id, domain, diff, ip, request_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10);

-- name: ListAuditOffset :many
SELECT id, actor_id, actor_email, action, resource_type, resource_id, domain, diff, ip, request_id, created_at
FROM audit_log
WHERE (sqlc.arg('actor')::text = '' OR actor_id = sqlc.arg('actor')::text)
  AND (sqlc.arg('action')::text = '' OR action = sqlc.arg('action')::text)
  AND (sqlc.arg('resource')::text = '' OR resource_type = sqlc.arg('resource')::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- name: DeleteAuditOlderThan :execrows
DELETE FROM audit_log WHERE created_at < sqlc.arg('cutoff');

-- name: DeleteDemoAudit :execrows
DELETE FROM audit_log WHERE id LIKE 'demo-%' OR actor_id LIKE 'demo-%';
