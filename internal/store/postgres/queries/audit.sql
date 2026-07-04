-- name: InsertAuditLog :exec
INSERT INTO app_audit_logs (
  id, category, subcategory, event_type, action, outcome,
  actor_user_id, actor_organisation_id, target_type, target_id, target_display,
  request_id, trace_id, span_id, ip_address, user_agent,
  metadata_json, changes_json, created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19);

-- name: ListAuditLogs :many
SELECT
  id, category, subcategory, event_type, action, outcome,
  actor_user_id, actor_organisation_id, target_type, target_id, target_display,
  request_id, trace_id, span_id, ip_address, user_agent,
  metadata_json, changes_json, created_at
FROM app_audit_logs
WHERE ($1 = '' OR category = $2)
  AND ($3 = '' OR subcategory = $4)
  AND ($5 = '' OR event_type = $6)
  AND ($7 = '' OR actor_user_id = $8)
  AND ($9 = '' OR target_type = $10)
  AND ($11 = '' OR target_id = $12)
  AND ($13 = '' OR outcome = $14)
  AND (sqlc.narg('before')::timestamptz IS NULL OR created_at <= sqlc.narg('before'))
ORDER BY created_at DESC, id DESC
LIMIT $15;
