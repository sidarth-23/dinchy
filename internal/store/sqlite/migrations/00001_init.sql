-- +goose Up

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  email_verified_at TEXT,
  disabled_at TEXT,
  last_login_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS accounts (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  provider TEXT NOT NULL,
  provider_account_id TEXT NOT NULL,
  password_hash TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(provider, provider_account_id),
  UNIQUE(user_id, provider),
  FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS organisations (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT NOT NULL UNIQUE,
  logo TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS organisation_members (
  id TEXT PRIMARY KEY,
  organisation_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  role TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(organisation_id, user_id),
  FOREIGN KEY(organisation_id) REFERENCES organisations(id),
  FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS organisation_invitations (
  id TEXT PRIMARY KEY,
  organisation_id TEXT NOT NULL,
  email TEXT NOT NULL,
  role TEXT NOT NULL,
  status TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TEXT NOT NULL,
  invited_by_user_id TEXT NOT NULL,
  accepted_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(organisation_id) REFERENCES organisations(id),
  FOREIGN KEY(invited_by_user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  active_organisation_id TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  ip_address TEXT NOT NULL,
  user_agent TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  idle_expires_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id),
  FOREIGN KEY(active_organisation_id) REFERENCES organisations(id)
);

CREATE TABLE IF NOT EXISTS verification_tokens (
  id TEXT PRIMARY KEY,
  user_id TEXT,
  email TEXT NOT NULL,
  purpose TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TEXT NOT NULL,
  consumed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS two_factors (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL UNIQUE,
  secret TEXT NOT NULL,
  verified INTEGER NOT NULL,
  last_used_step INTEGER,
  failed_verification_count INTEGER NOT NULL,
  locked_until TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS app_settings (
  id TEXT PRIMARY KEY,
  instance_name TEXT NOT NULL,
  session_idle_timeout_seconds INTEGER NOT NULL,
  session_max_lifetime_seconds INTEGER NOT NULL,
  session_cleanup_interval_seconds INTEGER NOT NULL,
  session_retention_seconds INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS auth_audit_logs (
  id TEXT PRIMARY KEY,
  event_type TEXT NOT NULL,
  user_id TEXT,
  actor_user_id TEXT,
  ip_address TEXT NOT NULL,
  user_agent TEXT NOT NULL,
  metadata_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS scheduled_tasks (
  id TEXT PRIMARY KEY,
  task_name TEXT NOT NULL UNIQUE,
  enabled INTEGER NOT NULL,
  schedule_interval_seconds INTEGER NOT NULL,
  lease_owner TEXT,
  lease_expires_at TEXT,
  last_run_at TEXT,
  last_finished_at TEXT,
  next_run_at TEXT,
  last_status TEXT,
  last_error_code TEXT,
  last_error_message TEXT,
  updated_at TEXT NOT NULL
);
