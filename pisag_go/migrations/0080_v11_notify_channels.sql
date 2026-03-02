BEGIN;

CREATE SCHEMA IF NOT EXISTS ops_v11;
REVOKE ALL ON SCHEMA ops_v11 FROM PUBLIC;

-- 通知先 (SoT)
CREATE TABLE IF NOT EXISTS ops_v11.notify_channels_v11 (
  id bigserial PRIMARY KEY,

  project_id varchar(26) NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

  channel_key text NOT NULL, -- human-friendly stable key (e.g. "slack_ops", "email_oncall")
  channel_type varchar(16) NOT NULL, -- slack|email|webhook
  destination_ref text NOT NULL,     -- destination identifier; secrets are stored elsewhere
  status varchar(16) NOT NULL DEFAULT 'active', -- active|paused

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT notify_channels_v11_key_nonempty CHECK (btrim(channel_key) <> ''),
  CONSTRAINT notify_channels_v11_type_valid CHECK (lower(channel_type) IN ('slack','email','webhook')),
  CONSTRAINT notify_channels_v11_status_valid CHECK (lower(status) IN ('active','paused')),

  CONSTRAINT ux_notify_channels_v11_project_key UNIQUE(project_id, channel_key)
);

CREATE INDEX IF NOT EXISTS idx_notify_channels_v11_project_status
  ON ops_v11.notify_channels_v11(project_id, status, updated_at DESC);

COMMIT;