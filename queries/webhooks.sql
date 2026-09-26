-- name: InsertWebhookEndpoint :exec
INSERT INTO webhook_endpoints (id, url, secret, events, description, active, org_id, created_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: UpdateWebhookEndpoint :exec
UPDATE webhook_endpoints
SET url = $2, events = $3, description = $4, active = $5, updated_at = now()
WHERE id = $1;

-- name: DeleteWebhookEndpoint :execrows
DELETE FROM webhook_endpoints WHERE id = $1;

-- name: GetWebhookEndpoint :one
SELECT id, url, secret, events, description, active, org_id, created_by, created_at, updated_at
FROM webhook_endpoints
WHERE id = $1;

-- name: ListWebhookEndpoints :many
SELECT id, url, secret, events, description, active, org_id, created_by, created_at, updated_at
FROM webhook_endpoints
ORDER BY created_at DESC;

-- name: InsertWebhookDelivery :exec
INSERT INTO webhook_deliveries (id, endpoint_id, event, payload, status, created_at)
VALUES ($1, $2, $3, $4::jsonb, $5, $6);

-- name: UpdateWebhookDelivery :exec
UPDATE webhook_deliveries
SET status = $2, attempts = $3, last_response_status = $4, last_error = $5, delivered_at = $6
WHERE id = $1;

-- name: ListWebhookDeliveries :many
SELECT id, endpoint_id, event, payload, status, attempts, last_response_status, last_error, delivered_at, created_at
FROM webhook_deliveries
WHERE endpoint_id = $1
ORDER BY created_at DESC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- name: GetWebhookDelivery :one
SELECT id, endpoint_id, event, payload, status, attempts, last_response_status, last_error, delivered_at, created_at
FROM webhook_deliveries
WHERE id = $1;

-- name: DeleteDemoWebhookEndpoints :execrows
DELETE FROM webhook_endpoints WHERE id LIKE 'demo-%';
