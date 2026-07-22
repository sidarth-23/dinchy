-- name: InsertSession :exec
INSERT INTO sessions (id, user_id, active_organization_id, token_hash, ip_address, user_agent, last_seen_at, idle_expires_at, expires_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: GetSessionByTokenHash :one
SELECT s.id, s.user_id, u.email, u.display_name, s.active_organization_id, o.name AS organization_name, o.slug AS organization_slug, m.role,
  COALESCE(array_agg(rp.permission) FILTER (WHERE rp.permission IS NOT NULL), '{}')::text[] AS permissions,
  s.idle_expires_at, s.expires_at, s.revoked_at
FROM sessions s
JOIN users u ON u.id = s.user_id
JOIN organizations o ON o.id = s.active_organization_id
JOIN organization_members m ON m.user_id = u.id AND m.organization_id = o.id
LEFT JOIN organization_roles r ON r.organization_id = o.id AND r.role_key = m.role
LEFT JOIN organization_role_permissions rp ON rp.role_id = r.id
WHERE s.token_hash = $1
GROUP BY s.id, u.id, o.id, m.role;

-- name: GetActiveSessionTokenHashesForUser :many
SELECT token_hash FROM sessions
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: GetActiveSessionTokenHashesForOrganization :many
SELECT token_hash FROM sessions
WHERE active_organization_id = $1 AND revoked_at IS NULL;

-- name: RevokeSessionByTokenHash :exec
UPDATE sessions
SET revoked_at = $1, updated_at = $2
WHERE token_hash = $3 AND revoked_at IS NULL;

-- name: DeleteEndedSessionsOlderThan :execresult
DELETE FROM sessions
WHERE (revoked_at IS NOT NULL OR expires_at < $1) AND updated_at < $2;
