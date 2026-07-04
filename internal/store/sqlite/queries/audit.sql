-- name: InsertAuditLog :exec
INSERT INTO app_audit_logs (
  id, category, subcategory, event_type, action, outcome,
  actor_user_id, actor_organisation_id, target_type, target_id, target_display,
  request_id, trace_id, span_id, ip_address, user_agent,
  metadata_json, changes_json, created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListAuditLogs :many
SELECT
  id, category, subcategory, event_type, action, outcome,
  actor_user_id, actor_organisation_id, target_type, target_id, target_display,
  request_id, trace_id, span_id, ip_address, user_agent,
  metadata_json, changes_json, created_at
FROM app_audit_logs
WHERE (? = '' OR category = ?)
  AND (? = '' OR subcategory = ?)
  AND (? = '' OR event_type = ?)
  AND (? = '' OR actor_user_id = ?)
  AND (? = '' OR target_type = ?)
  AND (? = '' OR target_id = ?)
  AND (? = '' OR outcome = ?)
  AND (? = '' OR created_at <= ?)
ORDER BY created_at DESC, id DESC
LIMIT ?;
