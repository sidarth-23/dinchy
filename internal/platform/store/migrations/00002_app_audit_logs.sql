-- +goose Up

CREATE TABLE IF NOT EXISTS app_audit_logs (
  id UUID PRIMARY KEY,
  category TEXT NOT NULL,
  subcategory TEXT NOT NULL,
  event_type TEXT NOT NULL,
  action TEXT NOT NULL,
  outcome TEXT NOT NULL,
  actor_user_id UUID,
  actor_organisation_id UUID,
  target_type TEXT,
  target_id TEXT,
  target_display TEXT,
  request_id TEXT,
  trace_id TEXT,
  span_id TEXT,
  ip_address TEXT NOT NULL,
  user_agent TEXT NOT NULL,
  metadata_json TEXT NOT NULL,
  changes_json TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS app_audit_logs_created_at_idx ON app_audit_logs(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS app_audit_logs_category_idx ON app_audit_logs(category, subcategory, created_at DESC);
CREATE INDEX IF NOT EXISTS app_audit_logs_actor_idx ON app_audit_logs(actor_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS app_audit_logs_target_idx ON app_audit_logs(target_type, target_id, created_at DESC);
