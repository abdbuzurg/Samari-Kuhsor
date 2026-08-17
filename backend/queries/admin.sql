-- Role management and the audit viewer — docs/04-RBAC.md §6.

-- name: ListRoles :many
SELECT r.*, (SELECT count(*) FROM user_roles ur
             WHERE ur.role_id=r.id AND ur.deleted_at IS NULL)::int AS user_count
FROM roles r WHERE r.deleted_at IS NULL ORDER BY r.is_system DESC, r.key;

-- name: GetRole :one
SELECT * FROM roles WHERE id=$1 AND deleted_at IS NULL;

-- name: CreateRole :one
INSERT INTO roles (key, name_ru, name_tg, name_en, created_by)
VALUES ($1,$2,$3,$4,$5) RETURNING *;

-- name: TombstoneRole :one
-- Seed roles are is_system and cannot be deleted (D9); the guard is in the WHERE
-- clause so it holds regardless of caller.
UPDATE roles SET deleted_at=now()
WHERE id=$1 AND deleted_at IS NULL AND is_system=false RETURNING *;

-- name: ListRolePermissions :many
SELECT * FROM role_permissions WHERE role_id=$1 AND deleted_at IS NULL
ORDER BY resource, action;

-- name: ClearRolePermissions :exec
UPDATE role_permissions SET deleted_at=now() WHERE role_id=$1 AND deleted_at IS NULL;

-- name: GrantPermission :exec
INSERT INTO role_permissions (role_id, resource, action, created_by)
VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING;

-- name: ListUsersWithRoles :many
SELECT u.id, u.email, u.full_name, u.is_active, u.last_login_at,
  COALESCE(ARRAY_AGG(r.key ORDER BY r.key) FILTER (WHERE r.id IS NOT NULL), '{}')::text[] AS role_keys
FROM users u
LEFT JOIN user_roles ur ON ur.user_id=u.id AND ur.deleted_at IS NULL
LEFT JOIN roles r ON r.id=ur.role_id AND r.deleted_at IS NULL
WHERE u.deleted_at IS NULL
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(u.full_name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR lower(u.email) LIKE '%'||lower(sqlc.narg(q))||'%')
GROUP BY u.id ORDER BY u.full_name LIMIT $1 OFFSET $2;

-- name: CountUsers :one
SELECT count(*) FROM users u WHERE u.deleted_at IS NULL
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(u.full_name)) LIKE '%'||unaccent(lower(sqlc.narg(q)))||'%'
       OR lower(u.email) LIKE '%'||lower(sqlc.narg(q))||'%');

-- name: ClearUserRoles :exec
UPDATE user_roles SET deleted_at=now() WHERE user_id=$1 AND deleted_at IS NULL;

-- name: AssignRole :exec
INSERT INTO user_roles (user_id, role_id, created_by)
VALUES ($1,$2,$3) ON CONFLICT DO NOTHING;

-- name: SetUserActive :one
UPDATE users SET is_active=$2 WHERE id=$1 AND deleted_at IS NULL RETURNING *;

-- name: CountUsersHoldingPermission :one
-- The last-admin guard (docs/04-RBAC.md:147). Counts ACTIVE, non-deleted users
-- holding a permission, because a deactivated administrator cannot rescue anyone.
SELECT count(DISTINCT u.id)::int
FROM users u
JOIN user_roles ur ON ur.user_id=u.id AND ur.deleted_at IS NULL
JOIN roles r ON r.id=ur.role_id AND r.deleted_at IS NULL
JOIN role_permissions rp ON rp.role_id=r.id AND rp.deleted_at IS NULL
WHERE u.deleted_at IS NULL AND u.is_active
  AND rp.resource=$1 AND rp.action=$2
  AND (sqlc.narg(excluding)::uuid IS NULL OR u.id <> sqlc.narg(excluding));
