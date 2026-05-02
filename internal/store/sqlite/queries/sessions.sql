-- name: InsertSession :exec
INSERT INTO sessions (id, user_id, token_hash, ip_address, user_agent, last_seen_at, idle_expires_at, expires_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetSessionByTokenHash :one
SELECT s.id, s.user_id, u.email, u.display_name, u.role, s.idle_expires_at, s.expires_at, s.revoked_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = ?;

-- name: RevokeSessionByTokenHash :exec
UPDATE sessions
SET revoked_at = ?, updated_at = ?
WHERE token_hash = ? AND revoked_at IS NULL;

-- name: DeleteEndedSessionsOlderThan :execresult
DELETE FROM sessions
WHERE (revoked_at IS NOT NULL OR expires_at < ?) AND updated_at < ?;
