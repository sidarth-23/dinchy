-- name: ListSSOProviderSettings :many
SELECT provider_id, client_id, client_secret, callback_url, enabled, created_at, updated_at
FROM sso_provider_settings
ORDER BY provider_id;

-- name: UpsertSSOProviderSetting :exec
INSERT INTO sso_provider_settings (
    provider_id,
    client_id,
    client_secret,
    callback_url,
    enabled,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT(provider_id) DO UPDATE SET
    client_id = excluded.client_id,
    client_secret = excluded.client_secret,
    callback_url = excluded.callback_url,
    enabled = excluded.enabled,
    updated_at = excluded.updated_at;
