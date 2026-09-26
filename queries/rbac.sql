-- Role metadata (the display half of RBAC; the permissions themselves live
-- in casbin_rule and are enforced by Casbin).

-- name: CreateRole :exec
INSERT INTO roles (key, name, description, is_system)
VALUES ($1, $2, $3, $4)
ON CONFLICT (key) DO NOTHING;

-- name: GetRole :one
SELECT key, name, description, is_system, created_at
FROM roles
WHERE key = $1;

-- name: UpdateRole :exec
UPDATE roles
SET name = $2, description = $3
WHERE key = $1;

-- name: DeleteRole :execrows
DELETE FROM roles WHERE key = $1;

-- name: ListRoles :many
SELECT key, name, description, is_system, created_at
FROM roles
ORDER BY created_at ASC, key ASC;

-- Casbin policy rules. ptype "p": v0 subject, v1 domain, v2 object, v3
-- action; ptype "g": v0 user, v1 role, v2 domain.

-- name: ListPolicyRules :many
SELECT ptype, v0, v1, v2, v3
FROM casbin_rule
WHERE ptype = sqlc.arg('ptype')
ORDER BY id ASC;

-- name: InsertPolicyRule :exec
INSERT INTO casbin_rule (ptype, v0, v1, v2, v3)
VALUES ($1, $2, $3, $4, $5);

-- name: DeletePolicyRule :execrows
DELETE FROM casbin_rule
WHERE ptype = $1 AND v0 = $2 AND v1 = $3 AND v2 = $4 AND v3 = $5;

-- name: DeletePolicyRulesByPrefix :execrows
DELETE FROM casbin_rule
WHERE ptype = $1 AND v0 = $2 AND v1 = $3;

-- name: DeleteAllPolicyRules :execrows
DELETE FROM casbin_rule WHERE ptype = $1;

-- name: DeleteDemoGroupRules :execrows
DELETE FROM casbin_rule WHERE ptype = 'g' AND v0 LIKE 'demo-%';

-- name: DeleteDemoOrgRules :execrows
DELETE FROM casbin_rule WHERE ptype = 'p' AND v1 LIKE 'demo-%';
