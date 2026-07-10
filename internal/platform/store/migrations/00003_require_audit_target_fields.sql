-- +goose Up

ALTER TABLE app_audit_logs
  ALTER COLUMN target_type SET NOT NULL,
  ALTER COLUMN target_id SET NOT NULL,
  ALTER COLUMN target_display SET NOT NULL;

-- +goose Down

ALTER TABLE app_audit_logs
  ALTER COLUMN target_type DROP NOT NULL,
  ALTER COLUMN target_id DROP NOT NULL,
  ALTER COLUMN target_display DROP NOT NULL;
