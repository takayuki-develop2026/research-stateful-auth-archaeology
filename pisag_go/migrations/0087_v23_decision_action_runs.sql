-- 0087_v23_decision_action_runs.sql
-- v23: Every apply is run-ified (v3 alignment). action execution is traceable/restartable.

BEGIN;

CREATE TABLE IF NOT EXISTS decision_action_runs_v23 (
  id BIGSERIAL PRIMARY KEY,

  project_id TEXT NOT NULL,

  action_id BIGINT NOT NULL REFERENCES decision_actions_v23(id) ON DELETE CASCADE,
  apply_run_id UUID NOT NULL,

  status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed_soft')),

  created_at_utc TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at_utc TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_dar23_action_apply_run
  ON decision_action_runs_v23(action_id, apply_run_id);

CREATE INDEX IF NOT EXISTS idx_dar23_project_created
  ON decision_action_runs_v23(project_id, created_at_utc DESC);

COMMIT;