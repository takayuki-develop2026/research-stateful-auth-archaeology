BEGIN;

CREATE SCHEMA IF NOT EXISTS agent_v10;
REVOKE ALL ON SCHEMA agent_v10 FROM PUBLIC;

-- ------------------------------------------------------------
-- routing_proposals_v10 (SoT)
-- project_id is TEXT (akproj_...)
-- all heavy bodies go to evidence_assets, only evidence_ref stored here
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS agent_v10.routing_proposals_v10 (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

  project_id varchar(26) NOT NULL,              -- FK -> projects(id)
  policy_set_id uuid NOT NULL,                  -- gov_policy.policy_sets.id
  policy_version_base uuid NULL,                -- gov_policy.policy_versions.id (optional)

  proposal_type varchar(32) NOT NULL,           -- weight_tune/rule_add/...
  risk_level varchar(16) NOT NULL,              -- low/medium/high

  change_set_evidence_ref uuid NOT NULL,        -- FK -> evidence_assets(project_id,evidence_ref)
  rationale_summary varchar(512) NOT NULL,
  rationale_evidence_ref uuid NOT NULL,         -- FK -> evidence_assets(project_id,evidence_ref)

  impact_summary jsonb NOT NULL DEFAULT '{}'::jsonb, -- small bounded JSON

  status varchar(32) NOT NULL,                  -- draft/evaluating/ready_for_review/...
  created_by_type varchar(16) NOT NULL,         -- system/user/service
  created_by_id varchar(128) NULL,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT routing_proposals_v10_project_fk
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE CASCADE,

  CONSTRAINT routing_proposals_v10_created_by_type_ck
    CHECK (created_by_type IN ('system','user','service')),

  CONSTRAINT routing_proposals_v10_risk_ck
    CHECK (risk_level IN ('low','medium','high')),

  CONSTRAINT routing_proposals_v10_status_ck
    CHECK (status IN ('draft','evaluating','ready_for_review','approved','rejected','review_required','failed'))
);

CREATE INDEX IF NOT EXISTS idx_routing_proposals_v10_project_status
  ON agent_v10.routing_proposals_v10(project_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_routing_proposals_v10_project_created
  ON agent_v10.routing_proposals_v10(project_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_routing_proposals_v10_project_policy_base
  ON agent_v10.routing_proposals_v10(project_id, policy_version_base);

-- evidence_refs must exist (project_id, evidence_ref)
ALTER TABLE agent_v10.routing_proposals_v10
  ADD CONSTRAINT fk_routing_proposals_v10_change_set_evidence
  FOREIGN KEY (project_id, change_set_evidence_ref)
  REFERENCES evidence_assets(project_id, evidence_ref) ON DELETE RESTRICT;

ALTER TABLE agent_v10.routing_proposals_v10
  ADD CONSTRAINT fk_routing_proposals_v10_rationale_evidence
  FOREIGN KEY (project_id, rationale_evidence_ref)
  REFERENCES evidence_assets(project_id, evidence_ref) ON DELETE RESTRICT;

COMMIT;