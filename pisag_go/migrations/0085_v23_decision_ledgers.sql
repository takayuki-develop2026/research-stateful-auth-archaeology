-- 0085_v23_decision_ledgers.sql
-- v23: Decision Ledger (ONLY SoT for decisions). asset-id-only.

BEGIN;

CREATE TABLE IF NOT EXISTS decision_ledgers_v23 (
  id BIGSERIAL PRIMARY KEY,

  project_id TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  run_id UUID NOT NULL,

  -- subject
  subject_type TEXT NOT NULL,
  subject_id TEXT NOT NULL,
  subject_owner_project_id TEXT NOT NULL,

  -- decision identity (deterministic)
  decision_key TEXT NOT NULL, -- sha256 hex
  decision_kind TEXT NOT NULL CHECK (decision_kind IN ('propose', 'approve', 'deny', 'edit_confirm', 'reject', 'override')),
  decision_scope TEXT NOT NULL CHECK (decision_scope IN ('managed', 'external')),

  policy_version_str TEXT NOT NULL,
  pipeline_version TEXT NOT NULL,

  input_hash TEXT NOT NULL, -- sha256 hex

  -- asset-id-only
  inputs_evidence_asset_id BIGINT NOT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT,
  proposal_evidence_asset_id BIGINT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT,
  obligations_evidence_asset_id BIGINT NOT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT,

  policy_evaluation_id BIGINT NULL REFERENCES policy_evaluations_v23(id) ON DELETE SET NULL,

  -- actor
  decided_by_type TEXT NOT NULL CHECK (decided_by_type IN ('system', 'human', 'service')),
  decided_by_id TEXT NOT NULL,
  decided_at_utc TIMESTAMPTZ NOT NULL DEFAULT now(),

  -- lifecycle
  status TEXT NOT NULL CHECK (status IN ('proposed', 'review_required', 'approved', 'denied', 'expired', 'superseded')),
  superseded_by_decision_id BIGINT NULL REFERENCES decision_ledgers_v23(id) ON DELETE SET NULL,
  expires_at_utc TIMESTAMPTZ NULL,

  -- optional human comment (asset-id-only)
  comment_evidence_asset_id BIGINT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_dl23_project_decision_key
  ON decision_ledgers_v23(project_id, decision_key);

CREATE INDEX IF NOT EXISTS idx_dl23_project_subject_status
  ON decision_ledgers_v23(project_id, subject_type, subject_id, status);

CREATE INDEX IF NOT EXISTS idx_dl23_trace
  ON decision_ledgers_v23(trace_id);

CREATE INDEX IF NOT EXISTS idx_dl23_run
  ON decision_ledgers_v23(run_id);

CREATE INDEX IF NOT EXISTS idx_dl23_project_created
  ON decision_ledgers_v23(project_id, decided_at_utc DESC);

COMMIT;