-- name: CreateOrg :exec
INSERT INTO organizations (id, name, slug, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO NOTHING;

-- name: GetOrg :one
SELECT id, name, slug, created_at, updated_at
FROM organizations
WHERE id = $1;

-- name: GetOrgBySlug :one
SELECT id, name, slug, created_at, updated_at
FROM organizations
WHERE slug = $1;

-- name: UpdateOrg :exec
UPDATE organizations
SET name = $2, slug = $3, updated_at = now()
WHERE id = $1;

-- name: DeleteOrg :execrows
DELETE FROM organizations WHERE id = $1;

-- name: ListOrgsOffset :many
SELECT id, name, slug, created_at, updated_at
FROM organizations
WHERE (sqlc.arg('query')::text = '' OR (name || ' ' || slug) ILIKE '%' || sqlc.arg('query')::text || '%')
ORDER BY created_at DESC, id ASC
LIMIT sqlc.arg('lim')
OFFSET sqlc.arg('off');

-- name: ListOrgsForUser :many
SELECT o.id, o.name, o.slug, o.created_at, o.updated_at
FROM organizations o
JOIN org_members m ON m.org_id = o.id
WHERE m.user_id = $1
ORDER BY o.name ASC;

-- name: CountOrgs :one
SELECT count(*) FROM organizations;

-- name: AddOrgMember :exec
INSERT INTO org_members (org_id, user_id, joined_at)
VALUES ($1, $2, $3)
ON CONFLICT (org_id, user_id) DO NOTHING;

-- name: RemoveOrgMember :execrows
DELETE FROM org_members WHERE org_id = $1 AND user_id = $2;

-- name: IsOrgMember :one
SELECT count(*) > 0 AS member
FROM org_members
WHERE org_id = $1 AND user_id = $2;

-- name: ListOrgMembers :many
SELECT m.org_id, m.user_id, u.email, u.name, m.joined_at
FROM org_members m
JOIN users u ON u.id = m.user_id
WHERE m.org_id = $1
ORDER BY m.joined_at ASC, m.user_id ASC;

-- name: DeleteDemoOrgs :execrows
DELETE FROM organizations WHERE id LIKE 'demo-%';
