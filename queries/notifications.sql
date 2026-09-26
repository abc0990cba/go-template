-- name: InsertNotification :exec
INSERT INTO notifications (id, user_id, type, title, body, link, read_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListNotifications :many
SELECT id, user_id, type, title, body, link, read_at, created_at
FROM notifications
WHERE user_id = $1
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- name: CountUnreadNotifications :one
SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkNotificationRead :execrows
UPDATE notifications SET read_at = now() WHERE id = $1 AND user_id = $2 AND read_at IS NULL;

-- name: MarkAllNotificationsRead :execrows
UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL;

-- name: GetNotification :one
SELECT id, user_id, type, title, body, link, read_at, created_at
FROM notifications
WHERE id = $1;

-- name: DeleteDemoNotifications :execrows
DELETE FROM notifications WHERE id LIKE 'demo-%' OR user_id LIKE 'demo-%';
