-- name: InsertAuditLog :exec
INSERT INTO app_audit_logs (
  id, category, subcategory, event_type, action, outcome,
  actor_user_id, actor_organization_id, target_type, target_id, target_display,
  request_id, trace_id, span_id, ip_address, user_agent,
  metadata_json, changes_json, created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19);

-- name: ListAuditLogs :many
SELECT
  id, category, subcategory, event_type, action, outcome,
  actor_user_id, actor_organization_id, target_type, target_id, target_display,
  request_id, trace_id, span_id, ip_address, user_agent,
  metadata_json, changes_json, created_at
FROM app_audit_logs
WHERE (sqlc.arg('category_filter')::text = '' OR category = sqlc.arg('category'))
  AND (sqlc.arg('subcategory_filter')::text = '' OR subcategory = sqlc.arg('subcategory'))
  AND (sqlc.arg('event_type_filter')::text = '' OR event_type = sqlc.arg('event_type'))
  AND (sqlc.arg('actor_user_id_filter')::text = '' OR actor_user_id = sqlc.arg('actor_user_id'))
  AND (sqlc.arg('target_type_filter')::text = '' OR target_type = sqlc.arg('target_type'))
  AND (sqlc.arg('target_id_filter')::text = '' OR target_id = sqlc.arg('target_id'))
  AND (sqlc.arg('outcome_filter')::text = '' OR outcome = sqlc.arg('outcome'))
  AND (sqlc.narg('before')::timestamptz IS NULL OR created_at <= sqlc.narg('before'))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('limit');
