-- name: CreateEmailMessage :exec
INSERT INTO email_messages (id, to_address, subject, body, kind, status, attempts, last_error, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetEmailMessage :one
SELECT id, to_address, subject, body, kind, status, attempts, last_error, created_at, sent_at
FROM email_messages
WHERE id = $1;

-- name: ListEmailMessages :many
SELECT id, to_address, subject, body, kind, status, attempts, last_error, created_at, sent_at
FROM email_messages
WHERE ($1::text = '' OR status = $1)
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateEmailMessageStatus :execrows
UPDATE email_messages
SET status = $2, attempts = $3, last_error = $4, sent_at = $5
WHERE id = $1;

-- name: DeleteDemoEmailMessages :execrows
DELETE FROM email_messages WHERE id LIKE 'demo-%' OR to_address LIKE 'demo-%';

-- name: PruneEmailMessages :execrows
DELETE FROM email_messages WHERE created_at < $1;
