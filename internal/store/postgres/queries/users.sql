-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: InsertUser :exec
INSERT INTO users (id, email, password_hash, display_name, role, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: FindUserByEmail :one
SELECT id, email, password_hash, display_name, role
FROM users
WHERE email = $1 AND disabled_at IS NULL;

