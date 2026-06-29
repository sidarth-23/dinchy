-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: InsertUser :exec
INSERT INTO users (id, email, display_name, email_verified_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: InsertAccount :exec
INSERT INTO accounts (id, user_id, provider, provider_account_id, password_hash, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: InsertOrganisation :exec
INSERT INTO organisations (id, name, slug, logo, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: InsertOrganisationMember :exec
INSERT INTO organisation_members (id, organisation_id, user_id, role, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: FindUserByEmail :one
SELECT id, email, display_name, disabled_at
FROM users
WHERE email = ? AND disabled_at IS NULL;

-- name: FindPasswordAccountByUserID :one
SELECT id, user_id, provider, provider_account_id, password_hash
FROM accounts
WHERE user_id = ? AND provider = 'password';

-- name: FindUserByProviderAccount :one
SELECT u.id, u.email, u.display_name, u.disabled_at
FROM accounts a
JOIN users u ON u.id = a.user_id
WHERE a.provider = ? AND a.provider_account_id = ? AND u.disabled_at IS NULL;

-- name: ListOrganisationsForUser :many
SELECT o.id, o.name, o.slug, m.role
FROM organisation_members m
JOIN organisations o ON o.id = m.organisation_id
WHERE m.user_id = ?
ORDER BY o.name;

-- name: FindOrganisationBySlugForUser :one
SELECT o.id, o.name, o.slug, m.role
FROM organisation_members m
JOIN organisations o ON o.id = m.organisation_id
WHERE m.user_id = ? AND o.slug = ?;

-- name: FindOrganisationByIDForUser :one
SELECT o.id, o.name, o.slug, m.role
FROM organisation_members m
JOIN organisations o ON o.id = m.organisation_id
WHERE m.user_id = ? AND o.id = ?;

-- name: FindPendingInvitationByEmailAndOrganisation :one
SELECT id, organisation_id, email, role, expires_at
FROM organisation_invitations
WHERE email = ? AND organisation_id = ? AND status = 'pending' AND accepted_at IS NULL;

-- name: InsertVerificationToken :exec
INSERT INTO verification_tokens (id, user_id, email, purpose, token_hash, expires_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: FindVerificationToken :one
SELECT id, user_id, email, purpose, token_hash, expires_at, consumed_at
FROM verification_tokens
WHERE token_hash = ? AND purpose = ?;

-- name: ConsumeVerificationToken :exec
UPDATE verification_tokens
SET consumed_at = ?, updated_at = ?
WHERE id = ? AND consumed_at IS NULL;

-- name: UpdateUserPasswordHash :exec
UPDATE accounts
SET password_hash = ?, updated_at = ?
WHERE user_id = ? AND provider = 'password';

-- name: InsertOrReplaceTwoFactor :exec
INSERT INTO two_factors (id, user_id, secret, verified, failed_verification_count, created_at, updated_at)
VALUES (?, ?, ?, ?, 0, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
  secret = excluded.secret,
  verified = excluded.verified,
  failed_verification_count = 0,
  locked_until = NULL,
  updated_at = excluded.updated_at;

-- name: FindTwoFactorByUserID :one
SELECT id, user_id, secret, verified, last_used_step, failed_verification_count, locked_until
FROM two_factors
WHERE user_id = ?;

-- name: ConfirmTwoFactor :exec
UPDATE two_factors
SET verified = 1, last_used_step = ?, failed_verification_count = 0, locked_until = NULL, updated_at = ?
WHERE user_id = ?;

-- name: DisableTwoFactor :exec
DELETE FROM two_factors
WHERE user_id = ?;

-- name: MarkTwoFactorUsed :exec
UPDATE two_factors
SET last_used_step = ?, failed_verification_count = 0, locked_until = NULL, updated_at = ?
WHERE user_id = ?;

-- name: RevokeSessionsForUser :exec
UPDATE sessions
SET revoked_at = ?, updated_at = ?
WHERE user_id = ? AND revoked_at IS NULL;
