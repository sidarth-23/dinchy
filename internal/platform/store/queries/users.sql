-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: InsertUser :exec
INSERT INTO users (id, email, display_name, email_verified_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: InsertAccount :exec
INSERT INTO accounts (id, user_id, provider, provider_account_id, password_hash, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: InsertOrganization :exec
INSERT INTO organizations (id, name, slug, logo, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: InsertOrganizationRole :exec
INSERT INTO organization_roles (id, organization_id, role_key, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5);

-- name: InsertOrganizationRolePermission :exec
INSERT INTO organization_role_permissions (role_id, permission)
VALUES ($1, $2);

-- name: InsertOrganizationMember :exec
INSERT INTO organization_members (id, organization_id, user_id, role, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: FindUserByEmail :one
SELECT id, email, display_name, email_verified_at, disabled_at
FROM users
WHERE email = $1 AND disabled_at IS NULL;

-- name: FindPasswordAccountByUserID :one
SELECT id, user_id, provider, provider_account_id, password_hash
FROM accounts
WHERE user_id = $1 AND provider = 'password';

-- name: FindUserByProviderAccount :one
SELECT u.id, u.email, u.display_name, u.email_verified_at, u.disabled_at
FROM accounts a
JOIN users u ON u.id = a.user_id
WHERE a.provider = $1 AND a.provider_account_id = $2 AND u.disabled_at IS NULL;

-- name: UpdateUserEmailVerifiedAt :exec
UPDATE users
SET email_verified_at = $1, updated_at = $2
WHERE id = $3;

-- name: ListOrganizationsForUser :many
SELECT o.id, o.name, o.slug, m.role
FROM organization_members m
JOIN organizations o ON o.id = m.organization_id
WHERE m.user_id = $1
ORDER BY o.name;

-- name: FindOrganizationBySlugForUser :one
SELECT o.id, o.name, o.slug, m.role
FROM organization_members m
JOIN organizations o ON o.id = m.organization_id
WHERE m.user_id = $1 AND o.slug = $2;

-- name: FindOrganizationByIDForUser :one
SELECT o.id, o.name, o.slug, m.role
FROM organization_members m
JOIN organizations o ON o.id = m.organization_id
WHERE m.user_id = $1 AND o.id = $2;

-- name: InsertVerificationToken :exec
INSERT INTO verification_tokens (id, user_id, email, purpose, token_hash, expires_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: FindVerificationToken :one
SELECT id, user_id, email, purpose, token_hash, expires_at, consumed_at
FROM verification_tokens
WHERE token_hash = $1 AND purpose = $2;

-- name: ConsumeVerificationToken :exec
UPDATE verification_tokens
SET consumed_at = $1, updated_at = $2
WHERE id = $3 AND consumed_at IS NULL;

-- name: InsertOrganizationInvitation :exec
INSERT INTO organization_invitations (
  id,
  organization_id,
  email,
  role,
  status,
  token_hash,
  expires_at,
  invited_by_user_id,
  created_at,
  updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: FindOrganizationInvitationByToken :one
SELECT id, organization_id, email, role, status, token_hash, expires_at, invited_by_user_id, accepted_at
FROM organization_invitations
WHERE token_hash = $1;

-- name: FindPendingOrganizationInvitationByEmail :one
SELECT id, organization_id, email, role, status, token_hash, expires_at, invited_by_user_id, accepted_at
FROM organization_invitations
WHERE organization_id = $1 AND email = $2 AND status = 'pending' AND accepted_at IS NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: ConsumeOrganizationInvitation :exec
UPDATE organization_invitations
SET status = 'accepted', accepted_at = $1, updated_at = $2
WHERE id = $3 AND status = 'pending' AND accepted_at IS NULL;

-- name: UpdateUserPasswordHash :exec
UPDATE accounts
SET password_hash = $1, updated_at = $2
WHERE user_id = $3 AND provider = 'password';

-- name: InsertOrReplaceTwoFactor :exec
INSERT INTO two_factors (id, user_id, secret, verified, failed_verification_count, created_at, updated_at)
VALUES ($1, $2, $3, $4, 0, $5, $6)
ON CONFLICT(user_id) DO UPDATE SET
  secret = excluded.secret,
  verified = excluded.verified,
  failed_verification_count = 0,
  locked_until = NULL,
  updated_at = excluded.updated_at;

-- name: FindTwoFactorByUserID :one
SELECT id, user_id, secret, verified, last_used_step, failed_verification_count, locked_until
FROM two_factors
WHERE user_id = $1;

-- name: ConfirmTwoFactor :exec
UPDATE two_factors
SET verified = true, last_used_step = $1, failed_verification_count = 0, locked_until = NULL, updated_at = $2
WHERE user_id = $3;

-- name: DisableTwoFactor :exec
DELETE FROM two_factors
WHERE user_id = $1;

-- name: MarkTwoFactorUsed :exec
UPDATE two_factors
SET last_used_step = $1, failed_verification_count = 0, locked_until = NULL, updated_at = $2
WHERE user_id = $3;

-- name: RegisterTwoFactorFailure :exec
UPDATE two_factors
SET
  failed_verification_count = failed_verification_count + 1,
  locked_until = CASE
    WHEN failed_verification_count + 1 >= sqlc.arg('failure_limit') THEN sqlc.arg('locked_until')
    ELSE locked_until
  END,
  updated_at = sqlc.arg('updated_at')
WHERE user_id = sqlc.arg('user_id');

-- name: RevokeSessionsForUser :exec
UPDATE sessions
SET revoked_at = $1, updated_at = $2
WHERE user_id = $3 AND revoked_at IS NULL;
