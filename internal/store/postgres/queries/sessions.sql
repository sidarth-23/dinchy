-- name: InsertSession :exec
INSERT INTO sessions (id, user_id, token_hash, ip_address, user_agent, last_seen_at, idle_expires_at, expires_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: GetSessionByTokenHash :one
SELECT s.id, s.user_id, u.email, u.display_name, u.role, s.idle_expires_at, s.expires_at, s.revoked_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1;

-- name: RevokeSessionByTokenHash :exec
UPDATE sessions
SET revoked_at = $1, updated_at = $2
WHERE token_hash = $3 AND revoked_at IS NULL;

-- name: DeleteEndedSessionsOlderThan :execresult
DELETE FROM sessions
WHERE (revoked_at IS NOT NULL OR expires_at < $1) AND updated_at < $2;

