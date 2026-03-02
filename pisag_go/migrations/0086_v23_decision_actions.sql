-- 0086_v23_decision_actions.sql
-- v23: Actions bound to decisions. Apply SoT is "status + evidence + run_events". asset-id-only.

BEGIN;

CREATE TABLE IF NOT EXISTS decision_actions_v23 (
  id BIGSERIAL PRIMARY KEY,

  project_id TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  run_id UUID NOT NULL,

  decision_ledger_id BIGINT NOT NULL REFERENCES decision_ledgers_v23(id) ON DELETE CASCADE,

  action_key TEXT NOT NULL, -- sha256 hex

  action_type TEXT NOT NULL,
  action_scope TEXT NOT NULL CHECK (action_scope IN ('managed', 'external')),

  -- target/plan (asset-id-only)
  target_hash TEXT NOT NULL, -- sha256 hex of canonical target
  target_evidence_asset_id BIGINT NOT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT,
  plan_evidence_asset_id BIGINT NOT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT,

  -- budget (v3.1/v20 integration points)
  budget_currency TEXT NOT NULL,
  budget_estimate_amount BIGINT NOT NULL,

  budget_reserved_id BIGINT NULL,
  budget_spent_id BIGINT NULL,
  budget_released_id BIGINT NULL,

  status TEXT NOT NULL CHECK (status IN (
    'queued', 'running', 'succeeded',
    'failed_soft', 'skipped_budget', 'blocked_policy', 'review_required'
  )),

  started_at_utc TIMESTAMPTZ NULL,
  finished_at_utc TIMESTAMPTZ NULL,

  error_evidence_asset_id BIGINT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_da23_project_action_key
  ON decision_actions_v23(project_id, action_key);

CREATE INDEX IF NOT EXISTS idx_da23_decision
  ON decision_actions_v23(decision_ledger_id);

CREATE INDEX IF NOT EXISTS idx_da23_project_status_finished
  ON decision_actions_v23(project_id, status, finished_at_utc DESC);

CREATE INDEX IF NOT EXISTS idx_da23_trace
  ON decision_actions_v23(trace_id);

COMMIT;