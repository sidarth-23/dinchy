-- name: EnsureTask :exec
INSERT INTO scheduled_tasks (id, task_name, enabled, schedule_interval_seconds, next_run_at, updated_at)
VALUES ($1, $2, TRUE, $3, $4, $5)
ON CONFLICT(task_name) DO NOTHING;

-- name: ClaimTask :execresult
UPDATE scheduled_tasks
SET lease_owner = $1, lease_expires_at = $2, last_run_at = $3, updated_at = $4
WHERE task_name = $5
  AND enabled = TRUE
  AND (lease_expires_at IS NULL OR lease_expires_at < $6)
  AND (next_run_at IS NULL OR next_run_at <= $7);

-- name: FinishTask :exec
UPDATE scheduled_tasks
SET
    lease_owner       = NULL,
    lease_expires_at  = NULL,
    last_finished_at  = $1,
    next_run_at       = $2,
    last_status       = $3,
    last_error_code   = $4,
    last_error_message = $5,
    updated_at        = $6
WHERE task_name = $7;

