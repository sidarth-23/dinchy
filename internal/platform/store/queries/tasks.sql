-- name: EnsureTask :exec
INSERT INTO scheduled_tasks (id, task_name, enabled, schedule_interval_seconds, next_run_at, updated_at)
VALUES (
  sqlc.arg('id'),
  sqlc.arg('task_name'),
  TRUE,
  sqlc.arg('schedule_interval_seconds'),
  sqlc.arg('next_run_at'),
  sqlc.arg('updated_at')
)
ON CONFLICT(task_name) DO NOTHING;

-- name: ClaimTask :execresult
UPDATE scheduled_tasks
SET lease_owner = sqlc.arg('lease_owner'),
    lease_expires_at = sqlc.arg('lease_expires_at'),
    last_run_at = sqlc.arg('last_run_at'),
    updated_at = sqlc.arg('updated_at')
WHERE task_name = sqlc.arg('task_name')
  AND enabled = TRUE
  AND (lease_expires_at IS NULL OR lease_expires_at < sqlc.arg('lease_expires_at_cutoff'))
  AND (next_run_at IS NULL OR next_run_at <= sqlc.arg('next_run_at_cutoff'));

-- name: FinishTask :exec
UPDATE scheduled_tasks
SET
    lease_owner        = NULL,
    lease_expires_at   = NULL,
    last_finished_at   = sqlc.arg('last_finished_at'),
    next_run_at        = sqlc.arg('next_run_at'),
    last_status        = sqlc.arg('last_status'),
    last_error_code    = sqlc.arg('last_error_code'),
    last_error_message = sqlc.arg('last_error_message'),
    updated_at         = sqlc.arg('updated_at')
WHERE task_name = sqlc.arg('task_name');
