-- +goose Up

DROP TABLE IF EXISTS scheduled_tasks;

-- +goose Down

CREATE TABLE IF NOT EXISTS scheduled_tasks (
  id UUID PRIMARY KEY,
  task_name TEXT NOT NULL UNIQUE,
  enabled BOOLEAN NOT NULL,
  schedule_interval_seconds BIGINT NOT NULL,
  lease_owner TEXT,
  lease_expires_at TIMESTAMPTZ,
  last_run_at TIMESTAMPTZ,
  last_finished_at TIMESTAMPTZ,
  next_run_at TIMESTAMPTZ,
  last_status TEXT,
  last_error_code TEXT,
  last_error_message TEXT,
  updated_at TIMESTAMPTZ NOT NULL
);
