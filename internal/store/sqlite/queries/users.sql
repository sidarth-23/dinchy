-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: InsertUser :exec
INSERT INTO users (id, email, password_hash, display_name, role, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: FindUserByEmail :one
SELECT id, email, password_hash, display_name, role
FROM users
WHERE email = ? AND disabled_at IS NULL;

-- name: UpdateUserPasswordHash :exec
UPDATE users
SET password_hash = ?, updated_at = ?
WHERE id = ?;
