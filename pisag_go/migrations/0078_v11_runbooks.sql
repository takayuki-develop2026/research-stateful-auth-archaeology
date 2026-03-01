BEGIN;

CREATE SCHEMA IF NOT EXISTS ops_v11;
REVOKE ALL ON SCHEMA ops_v11 FROM PUBLIC;

-- Runbooks SoT (v11.0)
-- project_id is text (varchar(26)), evidence is referenced by evidence_ref (uuid)
CREATE TABLE IF NOT EXISTS ops_v11.runbooks_v11 (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

  project_id varchar(26) NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

  runbook_key varchar(64) NOT NULL,
  title varchar(128) NOT NULL,

  steps_evidence_ref uuid NOT NULL,
  safety_checks_evidence_ref uuid NOT NULL,

  required_roles text[] NOT NULL DEFAULT ARRAY[]::text[],
  status varchar(16) NOT NULL DEFAULT 'active',

  created_by_type varchar(16) NOT NULL DEFAULT 'system',
  created_by_id varchar(128) NULL,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT runbooks_v11_status_ck CHECK (status IN ('active','deprecated')),
  CONSTRAINT runbooks_v11_created_by_type_ck CHECK (created_by_type IN ('system','user','service')),

  -- evidence_refs must exist for project
  CONSTRAINT fk_runbooks_steps_evidence
    FOREIGN KEY (project_id, steps_evidence_ref)
    REFERENCES evidence_assets(project_id, evidence_ref) ON DELETE RESTRICT,

  CONSTRAINT fk_runbooks_safety_evidence
    FOREIGN KEY (project_id, safety_checks_evidence_ref)
    REFERENCES evidence_assets(project_id, evidence_ref) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_runbooks_v11_project_key
  ON ops_v11.runbooks_v11(project_id, runbook_key);

CREATE INDEX IF NOT EXISTS idx_runbooks_v11_project_status
  ON ops_v11.runbooks_v11(project_id, status, updated_at DESC);

COMMIT;