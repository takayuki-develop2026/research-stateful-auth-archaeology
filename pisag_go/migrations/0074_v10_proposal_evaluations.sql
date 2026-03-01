BEGIN;

-- ------------------------------------------------------------
-- proposal_evaluations_v10 (SoT)
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS agent_v10.proposal_evaluations_v10 (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

  project_id varchar(26) NOT NULL,
  proposal_id uuid NOT NULL REFERENCES agent_v10.routing_proposals_v10(id) ON DELETE CASCADE,

  evaluation_type varchar(32) NOT NULL,         -- offline_replay (v10.0)
  dataset_evidence_ref uuid NOT NULL,
  metrics_evidence_ref uuid NOT NULL,

  metrics_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
  guard_summary jsonb NOT NULL DEFAULT '{}'::jsonb,

  status varchar(32) NOT NULL,                  -- queued/running/succeeded/review_required/failed
  trace_id varchar(64) NOT NULL,

  started_at timestamptz NULL,
  finished_at timestamptz NULL,

  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT proposal_eval_v10_project_fk
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,

  CONSTRAINT proposal_eval_v10_status_ck
    CHECK (status IN ('queued','running','succeeded','review_required','failed')),

  CONSTRAINT proposal_eval_v10_type_ck
    CHECK (evaluation_type IN ('offline_replay'))
);

CREATE INDEX IF NOT EXISTS idx_proposal_eval_v10_project_status
  ON agent_v10.proposal_evaluations_v10(project_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_proposal_eval_v10_project_proposal
  ON agent_v10.proposal_evaluations_v10(project_id, proposal_id);

-- evidence refs (project_id, evidence_ref)
ALTER TABLE agent_v10.proposal_evaluations_v10
  ADD CONSTRAINT fk_proposal_eval_v10_dataset_evidence
  FOREIGN KEY (project_id, dataset_evidence_ref)
  REFERENCES evidence_assets(project_id, evidence_ref) ON DELETE RESTRICT;

ALTER TABLE agent_v10.proposal_evaluations_v10
  ADD CONSTRAINT fk_proposal_eval_v10_metrics_evidence
  FOREIGN KEY (project_id, metrics_evidence_ref)
  REFERENCES evidence_assets(project_id, evidence_ref) ON DELETE RESTRICT;

COMMIT;