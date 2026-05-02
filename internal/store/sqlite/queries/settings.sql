-- name: EnsureDefaultSettings :exec
INSERT INTO app_settings (
    id,
    instance_name,
    session_idle_timeout_seconds,
    session_max_lifetime_seconds,
    session_cleanup_interval_seconds,
    session_retention_seconds,
    created_at,
    updated_at
) VALUES ('app_settings', 'Dinchy', 1800, 604800, 300, 86400, ?, ?)
ON CONFLICT(id) DO NOTHING;

-- name: GetInstanceName :one
SELECT instance_name FROM app_settings WHERE id = 'app_settings';
