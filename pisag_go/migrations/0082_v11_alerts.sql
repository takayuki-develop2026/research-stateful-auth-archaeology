BEGIN;

CREATE SCHEMA IF NOT EXISTS ops_v11;
REVOKE ALL ON SCHEMA ops_v11 FROM PUBLIC;

-- 発火ログ (SoT)
-- “小さいコンテキスト”は列に、詳細は evidence_assets.id に退避
CREATE TABLE IF NOT EXISTS ops_v11.alerts_v11 (
  id bigserial PRIMARY KEY,

  project_id varchar(26) NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  rule_id bigint NOT NULL REFERENCES ops_v11.alert_rules_v11(id) ON DELETE CASCADE,

  severity varchar(16) NOT NULL, -- copied from rule at fire time
  status varchar(16) NOT NULL DEFAULT 'open', -- open|acknowledged|resolved

  fired_at_utc timestamptz NOT NULL DEFAULT now(),
  resolved_at_utc timestamptz NULL,

  -- dedupe key expanded at fire time
  dedupe_key text NOT NULL,

  -- small context columns (avoid large json)
  trace_id text NULL,
  run_id uuid NULL,
  policy_set_id uuid NULL,
  policy_version_id uuid NULL,
  provider_hint varchar(64) NULL,

  context_evidence_asset_id bigint NOT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT,
  related_evidence_asset_ids bigint[] NOT NULL DEFAULT ARRAY[]::bigint[],

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT alerts_v11_severity_valid CHECK (lower(severity) IN ('info','warn','critical')),
  CONSTRAINT alerts_v11_status_valid CHECK (lower(status) IN ('open','acknowledged','resolved')),
  CONSTRAINT alerts_v11_dedupe_nonempty CHECK (btrim(dedupe_key) <> ''),
  CONSTRAINT alerts_v11_related_ids_cap CHECK (cardinality(related_evidence_asset_ids) <= 50)
);

CREATE INDEX IF NOT EXISTS idx_alerts_v11_project_status_time
  ON ops_v11.alerts_v11(project_id, status, fired_at_utc DESC);

CREATE INDEX IF NOT EXISTS idx_alerts_v11_project_rule_time
  ON ops_v11.alerts_v11(project_id, rule_id, fired_at_utc DESC);

CREATE INDEX IF NOT EXISTS idx_alerts_v11_project_dedupe_time
  ON ops_v11.alerts_v11(project_id, dedupe_key, fired_at_utc DESC);

COMMIT;