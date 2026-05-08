BEGIN;

CREATE SCHEMA IF NOT EXISTS ops_v11;
REVOKE ALL ON SCHEMA ops_v11 FROM PUBLIC;

-- アラート規則 (SoT)
-- condition は evidence_assets.id (bigint) に退避（本文/巨大JSONをSoTに置かない）
CREATE TABLE IF NOT EXISTS ops_v11.alert_rules_v11 (
  id bigserial PRIMARY KEY,

  project_id varchar(26) NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,

  rule_key text NOT NULL,              -- e.g. "router_success_drop"
  severity varchar(16) NOT NULL,       -- info|warn|critical
  status varchar(16) NOT NULL DEFAULT 'active', -- active|paused

  condition_evidence_asset_id bigint NOT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT,

  dedupe_key_template text NOT NULL,   -- template / hint; evaluator expands it
  cooldown_seconds int NOT NULL DEFAULT 300,

  -- notification targets (IDs only). FK for array is hard in SQL; enforce via app + cap.
  notify_channel_ids bigint[] NOT NULL DEFAULT ARRAY[]::bigint[],

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT alert_rules_v11_key_nonempty CHECK (btrim(rule_key) <> ''),
  CONSTRAINT alert_rules_v11_severity_valid CHECK (lower(severity) IN ('info','warn','critical')),
  CONSTRAINT alert_rules_v11_status_valid CHECK (lower(status) IN ('active','paused')),
  CONSTRAINT alert_rules_v11_cooldown_nonneg CHECK (cooldown_seconds >= 0),
  CONSTRAINT alert_rules_v11_notify_ids_cap CHECK (cardinality(notify_channel_ids) <= 20),

  CONSTRAINT ux_alert_rules_v11_project_key UNIQUE(project_id, rule_key)
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_v11_project_status
  ON ops_v11.alert_rules_v11(project_id, severity, status);

CREATE INDEX IF NOT EXISTS idx_alert_rules_v11_project_key
  ON ops_v11.alert_rules_v11(project_id, rule_key);

COMMIT;