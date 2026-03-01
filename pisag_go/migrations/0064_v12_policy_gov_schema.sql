BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE SCHEMA IF NOT EXISTS gov_policy;
REVOKE ALL ON SCHEMA gov_policy FROM PUBLIC;

-- ============================================================
-- policy_sets
-- ============================================================
CREATE TABLE IF NOT EXISTS gov_policy.policy_sets (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL,
  name varchar(64) NOT NULL,
  description text NULL,
  active_published_version_id uuid NULL,
  status text NOT NULL DEFAULT 'active',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT gov_policy_sets_status_chk CHECK (status IN ('active','archived'))
);

CREATE UNIQUE INDEX IF NOT EXISTS gov_policy_sets_uq_project_name
  ON gov_policy.policy_sets(project_id, name);

CREATE INDEX IF NOT EXISTS gov_policy_sets_idx_project_status
  ON gov_policy.policy_sets(project_id, status);

-- ============================================================
-- policy_versions (published/retired only)
-- ============================================================
CREATE TABLE IF NOT EXISTS gov_policy.policy_versions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  policy_set_id uuid NOT NULL REFERENCES gov_policy.policy_sets(id) ON DELETE CASCADE,
  version_number int NOT NULL,
  status text NOT NULL, -- published|retired
  compiled_policy_evidence_asset_id uuid NOT NULL, -- references existing evidence_assets (v18)
  compiled_policy_checksum char(64) NOT NULL,      -- sha256 hex
  published_by text NOT NULL,
  published_at timestamptz NOT NULL,
  publish_reason text NOT NULL,
  previous_version_id uuid NULL REFERENCES gov_policy.policy_versions(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT gov_policy_versions_status_chk CHECK (status IN ('published','retired'))
);

CREATE UNIQUE INDEX IF NOT EXISTS gov_policy_versions_uq_set_version
  ON gov_policy.policy_versions(policy_set_id, version_number);

CREATE INDEX IF NOT EXISTS gov_policy_versions_idx_set_status
  ON gov_policy.policy_versions(policy_set_id, status);

-- NOTE:
-- active_published_version_id must point to a published version.
-- Postgres can't do CHECK with cross-table lookup cleanly; enforce via exec-only fn.

-- ============================================================
-- policy_publications (publish/rollback/retire)
-- status vocab fixed: succeeded|failed_recorded
-- ============================================================
CREATE TABLE IF NOT EXISTS gov_policy.policy_publications (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL,
  policy_set_id uuid NOT NULL REFERENCES gov_policy.policy_sets(id) ON DELETE CASCADE,
  action text NOT NULL, -- publish|rollback|retire
  from_version_id uuid NULL REFERENCES gov_policy.policy_versions(id),
  to_version_id uuid NULL REFERENCES gov_policy.policy_versions(id),
  triggered_by text NOT NULL,
  triggered_at timestamptz NOT NULL DEFAULT now(),
  reason text NOT NULL,
  incident_id text NULL,
  status text NOT NULL, -- succeeded|failed_recorded
  result_evidence_asset_id uuid NULL, -- references evidence_assets (v18)
  trace_id text NOT NULL,
  idempotency_key text NOT NULL,
  CONSTRAINT gov_policy_publications_action_chk CHECK (action IN ('publish','rollback','retire')),
  CONSTRAINT gov_policy_publications_status_chk CHECK (status IN ('succeeded','failed_recorded')),
  CONSTRAINT gov_policy_publications_idem_nonempty_chk CHECK (length(trim(idempotency_key)) > 0),
  CONSTRAINT gov_policy_publications_trace_nonempty_chk CHECK (length(trim(trace_id)) > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS gov_policy_publications_uq_project_idem
  ON gov_policy.policy_publications(project_id, idempotency_key);

CREATE INDEX IF NOT EXISTS gov_policy_publications_idx_set_time
  ON gov_policy.policy_publications(policy_set_id, triggered_at DESC);

CREATE INDEX IF NOT EXISTS gov_policy_publications_idx_trace
  ON gov_policy.policy_publications(trace_id);

COMMIT;