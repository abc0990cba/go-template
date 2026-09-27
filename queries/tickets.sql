-- Ticket types: configurable approval chains (steps is a jsonb array of
-- {name, approver_role} objects).

-- name: InsertTicketType :exec
INSERT INTO ticket_types (id, org_id, key, name, description, steps, active, created_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10);

-- name: GetTicketType :one
SELECT id, org_id, key, name, description, steps, active, created_by, created_at, updated_at
FROM ticket_types
WHERE id = $1;

-- name: GetTicketTypeByKey :one
SELECT id, org_id, key, name, description, steps, active, created_by, created_at, updated_at
FROM ticket_types
WHERE org_id = $1 AND key = $2;

-- name: ListTicketTypes :many
SELECT id, org_id, key, name, description, steps, active, created_by, created_at, updated_at
FROM ticket_types
WHERE org_id = '' OR org_id = sqlc.arg('org')::text
ORDER BY key ASC;

-- name: UpdateTicketType :exec
UPDATE ticket_types
SET name = $2, description = $3, steps = $4::jsonb, active = $5, updated_at = now()
WHERE id = $1;

-- name: DeleteTicketType :execrows
DELETE FROM ticket_types WHERE id = $1;

-- Tickets. number is a per-organization sequence assigned at insert time.

-- name: InsertTicket :one
INSERT INTO tickets (id, number, org_id, type_id, type_key, title, description, priority, status, current_step, created_by, created_at, updated_at)
VALUES ($1, (SELECT COALESCE(MAX(number), 0) + 1 FROM tickets WHERE org_id = $2), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING number;

-- name: GetTicket :one
SELECT id, number, org_id, type_id, type_key, title, description, priority, status, current_step, created_by, decided_by, decided_at, created_at, updated_at
FROM tickets
WHERE id = $1;

-- name: UpdateTicket :exec
UPDATE tickets
SET title = $2, description = $3, priority = $4, status = $5, current_step = $6,
    decided_by = $7, decided_at = $8, updated_at = now()
WHERE id = $1;

-- name: DeleteTicket :execrows
DELETE FROM tickets WHERE id = $1;

-- name: ListTicketsOffset :many
SELECT id, number, org_id, type_id, type_key, title, description, priority, status, current_step, created_by, decided_by, decided_at, created_at, updated_at
FROM tickets
WHERE (sqlc.arg('query')::text = '' OR (title || ' ' || description) ILIKE '%' || sqlc.arg('query')::text || '%')
  AND (sqlc.arg('org')::text = '' OR org_id = sqlc.arg('org')::text)
  AND (sqlc.arg('creator')::text = '' OR created_by = sqlc.arg('creator')::text)
  AND (sqlc.arg('status')::text = '' OR status = sqlc.arg('status')::text)
  AND (sqlc.arg('typ')::text = '' OR type_key = sqlc.arg('typ')::text)
  AND (sqlc.arg('priority')::text = '' OR priority = sqlc.arg('priority')::text)
ORDER BY created_at DESC, id ASC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- name: ListTicketsKeyset :many
SELECT id, number, org_id, type_id, type_key, title, description, priority, status, current_step, created_by, decided_by, decided_at, created_at, updated_at
FROM tickets
WHERE (sqlc.arg('query')::text = '' OR (title || ' ' || description) ILIKE '%' || sqlc.arg('query')::text || '%')
  AND (sqlc.arg('org')::text = '' OR org_id = sqlc.arg('org')::text)
  AND (sqlc.arg('creator')::text = '' OR created_by = sqlc.arg('creator')::text)
  AND (sqlc.arg('status')::text = '' OR status = sqlc.arg('status')::text)
  AND (sqlc.arg('typ')::text = '' OR type_key = sqlc.arg('typ')::text)
  AND (sqlc.arg('priority')::text = '' OR priority = sqlc.arg('priority')::text)
  AND (
    created_at < (SELECT c.created_at FROM tickets c WHERE c.id = sqlc.arg('after'))
    OR (created_at = (SELECT c.created_at FROM tickets c WHERE c.id = sqlc.arg('after')) AND id > sqlc.arg('after'))
  )
ORDER BY created_at DESC, id ASC
LIMIT sqlc.arg('lim');

-- name: CountTicketsByStatus :many
SELECT status, count(*)::bigint AS n
FROM tickets
WHERE (sqlc.arg('org')::text = '' OR org_id = sqlc.arg('org')::text)
  AND (sqlc.arg('creator')::text = '' OR created_by = sqlc.arg('creator')::text)
GROUP BY status;

-- name: DeleteDemoTickets :execrows
DELETE FROM tickets WHERE id LIKE 'demo-%';

-- name: DeleteDemoTicketTypes :execrows
DELETE FROM ticket_types WHERE id LIKE 'demo-%';

-- name: DeleteTicketsReferencingDemoTypes :execrows
-- Anything else (e.g. tickets created against the seeded types) must go
-- before the types themselves or the foreign key blocks the demo reset.
DELETE FROM tickets WHERE type_id LIKE 'demo-%';

-- Approval steps: one row per step, created at submit time, decided in order.

-- name: InsertTicketApproval :exec
INSERT INTO ticket_approvals (id, ticket_id, step_index, name, approver_role, status, decided_by, decided_at, comment, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: DeleteTicketApprovals :exec
DELETE FROM ticket_approvals WHERE ticket_id = $1;

-- name: UpdateTicketApproval :exec
UPDATE ticket_approvals
SET status = $2, decided_by = $3, decided_at = $4, comment = $5
WHERE id = $1;

-- name: ListTicketApprovals :many
SELECT id, ticket_id, step_index, name, approver_role, status, decided_by, decided_at, comment, created_at
FROM ticket_approvals
WHERE ticket_id = $1
ORDER BY step_index ASC;

-- name: ListTicketsForApproverOffset :many
SELECT t.id, t.number, t.org_id, t.type_id, t.type_key, t.title, t.description, t.priority, t.status, t.current_step, t.created_by, t.decided_by, t.decided_at, t.created_at, t.updated_at
FROM tickets t
JOIN ticket_approvals a ON a.ticket_id = t.id AND a.step_index = t.current_step AND a.status = 'pending'
WHERE t.status = 'in_review'
  AND (sqlc.arg('org')::text = '' OR t.org_id = sqlc.arg('org')::text)
  AND a.approver_role = ANY(sqlc.arg('roles')::text[])
  AND (sqlc.arg('query')::text = '' OR (t.title || ' ' || t.description) ILIKE '%' || sqlc.arg('query')::text || '%')
ORDER BY t.created_at DESC, t.id ASC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- name: ListTicketsForApproverKeyset :many
SELECT t.id, t.number, t.org_id, t.type_id, t.type_key, t.title, t.description, t.priority, t.status, t.current_step, t.created_by, t.decided_by, t.decided_at, t.created_at, t.updated_at
FROM tickets t
JOIN ticket_approvals a ON a.ticket_id = t.id AND a.step_index = t.current_step AND a.status = 'pending'
WHERE t.status = 'in_review'
  AND (sqlc.arg('org')::text = '' OR t.org_id = sqlc.arg('org')::text)
  AND a.approver_role = ANY(sqlc.arg('roles')::text[])
  AND (sqlc.arg('query')::text = '' OR (t.title || ' ' || t.description) ILIKE '%' || sqlc.arg('query')::text || '%')
  AND (
    t.created_at < (SELECT c.created_at FROM tickets c WHERE c.id = sqlc.arg('after'))
    OR (t.created_at = (SELECT c.created_at FROM tickets c WHERE c.id = sqlc.arg('after')) AND t.id > sqlc.arg('after'))
  )
ORDER BY t.created_at DESC, t.id ASC
LIMIT sqlc.arg('lim');

-- name: CountTicketsForApprover :one
SELECT count(*)::bigint
FROM tickets t
JOIN ticket_approvals a ON a.ticket_id = t.id AND a.step_index = t.current_step AND a.status = 'pending'
WHERE t.status = 'in_review'
  AND (sqlc.arg('org')::text = '' OR t.org_id = sqlc.arg('org')::text)
  AND a.approver_role = ANY(sqlc.arg('roles')::text[]);

-- Decisions of one user, newest first, with the ticket snapshot attached.

-- name: ListDecidedApprovals :many
SELECT a.id, a.ticket_id, a.step_index, a.name, a.approver_role, a.status, a.decided_by, a.decided_at, a.comment, a.created_at,
       t.number, t.org_id, t.type_id, t.type_key, t.title, t.description, t.priority, t.status, t.current_step, t.created_by, t.decided_by, t.decided_at AS ticket_decided_at, t.created_at AS ticket_created_at, t.updated_at AS ticket_updated_at
FROM ticket_approvals a
JOIN tickets t ON t.id = a.ticket_id
WHERE a.decided_by = sqlc.arg('decided_by')::text
ORDER BY a.decided_at DESC, a.id ASC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- Comments.

-- name: InsertTicketComment :exec
INSERT INTO ticket_comments (id, ticket_id, author_id, body, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ListTicketComments :many
SELECT id, ticket_id, author_id, body, created_at
FROM ticket_comments
WHERE ticket_id = $1
ORDER BY created_at DESC, id ASC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- Timeline events.

-- name: InsertTicketEvent :exec
INSERT INTO ticket_events (id, ticket_id, actor_id, action, detail, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListTicketEvents :many
SELECT id, ticket_id, actor_id, action, detail, created_at
FROM ticket_events
WHERE ticket_id = $1
ORDER BY created_at DESC, id ASC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');
