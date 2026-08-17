-- Identity and session queries. Consumed by internal/auth (T05).
--
-- Every read filters deleted_at IS NULL (CLAUDE.md §4.3). The one exception is
-- session lookup by token hash, which must find revoked and expired sessions so
-- the domain layer can reject them with a precise reason instead of a generic 401.

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE lower(email) = lower(sqlc.arg(email)) AND deleted_at IS NULL;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: CreateUser :one
INSERT INTO users (email, full_name, password_hash, is_active, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: RecordLoginSuccess :exec
UPDATE users
SET last_login_at = now(), failed_attempts = 0, locked_until = NULL
WHERE id = $1 AND deleted_at IS NULL;

-- name: RecordLoginFailure :one
-- Increments the counter and locks the account once the threshold is reached.
-- Returning the row lets the caller report the resulting state without a re-read.
UPDATE users
SET failed_attempts = failed_attempts + 1,
    locked_until = CASE
      WHEN failed_attempts + 1 >= sqlc.arg(max_attempts)::int
      THEN now() + sqlc.arg(lockout)::interval
      ELSE locked_until
    END
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SetPasswordHash :exec
UPDATE users SET password_hash = $2 WHERE id = $1 AND deleted_at IS NULL;

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, expires_at, ip, user_agent, created_by)
VALUES ($1, $2, $3, $4, $5, $1)
RETURNING *;

-- name: GetSessionByTokenHash :one
-- Deliberately does not filter on expiry or revocation: the domain layer
-- distinguishes "unknown token" from "expired" from "revoked".
SELECT sqlc.embed(sessions), sqlc.embed(users)
FROM sessions
JOIN users ON users.id = sessions.user_id AND users.deleted_at IS NULL
WHERE sessions.token_hash = $1 AND sessions.deleted_at IS NULL;

-- name: TouchSession :exec
-- Idle-timeout support: slides the expiry window on activity.
UPDATE sessions SET expires_at = $2
WHERE id = $1 AND deleted_at IS NULL AND revoked_at IS NULL;

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = now()
WHERE id = $1 AND deleted_at IS NULL AND revoked_at IS NULL;

-- name: RevokeAllUserSessions :exec
-- Logout everywhere, required on password change (docs/03-API-CONTRACT.md:193).
UPDATE sessions SET revoked_at = now()
WHERE user_id = $1 AND deleted_at IS NULL AND revoked_at IS NULL;

-- name: DeleteExpiredSessions :exec
UPDATE sessions SET deleted_at = now()
WHERE deleted_at IS NULL AND expires_at < now() - sqlc.arg(retain)::interval;

-- name: GetUserPermissions :many
-- A user's effective permissions are the union across all their roles
-- (docs/04-RBAC.md §1). Resolved per request and never cached (04-RBAC.md:148).
SELECT DISTINCT rp.resource, rp.action
FROM user_roles ur
JOIN roles r ON r.id = ur.role_id AND r.deleted_at IS NULL
JOIN role_permissions rp ON rp.role_id = r.id AND rp.deleted_at IS NULL
WHERE ur.user_id = $1 AND ur.deleted_at IS NULL
ORDER BY rp.resource, rp.action;

-- name: GetUserRoles :many
SELECT r.* FROM user_roles ur
JOIN roles r ON r.id = ur.role_id AND r.deleted_at IS NULL
WHERE ur.user_id = $1 AND ur.deleted_at IS NULL
ORDER BY r.key;
