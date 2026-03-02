-- 0084_v23_policy_evaluations.sql
-- v23: PolicyEngine v2 evaluation summary (asset-id-only)
-- SoT rule: details are in evidence_assets; this table stores only references + indices.

BEGIN;

CREATE TABLE IF NOT EXISTS policy_evaluations_v23 (
  id BIGSERIAL PRIMARY KEY,

  project_id TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  run_id UUID NOT NULL,

  policy_version_str TEXT NOT NULL,
  pipeline_version TEXT NOT NULL,

  input_hash TEXT NOT NULL, -- sha256 hex

  pdp_mode TEXT NOT NULL CHECK (pdp_mode IN ('local', 'opa_remote', 'composite')),
  result TEXT NOT NULL CHECK (result IN ('allow', 'deny', 'review_required', 'proposal_only')),
  score NUMERIC NULL,

  -- asset-id-only
  reason_evidence_asset_id BIGINT NOT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT,
  obligations_evidence_asset_id BIGINT NOT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT,
  proposal_evidence_asset_id BIGINT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT,

  -- link to v21 policy_decisions (OPA detailed log) if present
  policy_decision_id BIGINT NULL,

  created_at_utc TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pev23_project_policy_created
  ON policy_evaluations_v23(project_id, policy_version_str, created_at_utc DESC);

CREATE INDEX IF NOT EXISTS idx_pev23_trace
  ON policy_evaluations_v23(trace_id);

CREATE INDEX IF NOT EXISTS idx_pev23_run
  ON policy_evaluations_v23(run_id);

CREATE INDEX IF NOT EXISTS idx_pev23_project_input
  ON policy_evaluations_v23(project_id, input_hash);

-- Optional safety net idempotency (recommended): same input + same policy/pipeline + same trace => reuse.
-- This complements Idempotency-Key at API layer.
CREATE UNIQUE INDEX IF NOT EXISTS uq_pev23_project_trace_input_policy_pipeline
  ON policy_evaluations_v23(project_id, trace_id, input_hash, policy_version_str, pipeline_version);

COMMIT;