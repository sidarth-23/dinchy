-- name: EnsureTask :exec
INSERT INTO scheduled_tasks (id, task_name, enabled, schedule_interval_seconds, next_run_at, updated_at)
VALUES (?, ?, 1, ?, ?, ?)
ON CONFLICT(task_name) DO NOTHING;

-- name: ClaimTask :execresult
UPDATE scheduled_tasks
SET lease_owner = ?, lease_expires_at = ?, last_run_at = ?, updated_at = ?
WHERE task_name = ?
  AND enabled = 1
  AND (lease_expires_at IS NULL OR lease_expires_at < ?)
  AND (next_run_at IS NULL OR next_run_at <= ?);

-- name: FinishTask :exec
UPDATE scheduled_tasks
SET
    lease_owner       = NULL,
    lease_expires_at  = NULL,
    last_finished_at  = ?,
    next_run_at       = ?,
    last_status       = ?,
    last_error_code   = ?,
    last_error_message = ?,
    updated_at        = ?
WHERE task_name = ?;
