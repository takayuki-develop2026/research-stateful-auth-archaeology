-- migrations/0061_v90_decision_ledger.sql
-- v9.0: decision_ledger_v9 (decision SoT)
-- Depends: 0060 engine_runs_v9, projects, evidence_assets, runs
BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.decision_ledger_v9 (
  decision_id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),

  project_id         varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  engine_run_id      uuid NOT NULL REFERENCES public.engine_runs_v9(engine_run_id) ON DELETE CASCADE,

  decision_type      varchar(24) NOT NULL, -- route|plan|proposal|reject|review_required

  result_json        jsonb NOT NULL DEFAULT '{}'::jsonb,       -- lightweight
  rationale_json     jsonb NOT NULL DEFAULT '{}'::jsonb,       -- lightweight
  constraints_json   jsonb NOT NULL DEFAULT '{}'::jsonb,       -- gate results (lightweight)

  decision_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_by_type    varchar(16) NOT NULL, -- system|user|service
  created_by_id      varchar(128) NULL,

  policy_version     varchar(32) NOT NULL,

  created_at         timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='decision_ledger_v9_type_ck') THEN
    ALTER TABLE public.decision_ledger_v9
      ADD CONSTRAINT decision_ledger_v9_type_ck CHECK (lower(decision_type::text) IN (
        'route','plan','proposal','reject','review_required'
      ));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='decision_ledger_v9_creator_ck') THEN
    ALTER TABLE public.decision_ledger_v9
      ADD CONSTRAINT decision_ledger_v9_creator_ck CHECK (lower(created_by_type::text) IN ('system','user','service'));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='decision_ledger_v9_policy_nonempty') THEN
    ALTER TABLE public.decision_ledger_v9
      ADD CONSTRAINT decision_ledger_v9_policy_nonempty CHECK (btrim(policy_version::text) <> '');
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_decision_ledger_v9_project_time
  ON public.decision_ledger_v9(project_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_decision_ledger_v9_project_type
  ON public.decision_ledger_v9(project_id, decision_type);

CREATE INDEX IF NOT EXISTS idx_decision_ledger_v9_project_run
  ON public.decision_ledger_v9(project_id, engine_run_id);

-- Back-link from engine_runs_v9 -> decision_id (FK)
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='engine_runs_v9_decision_fk') THEN
    ALTER TABLE public.engine_runs_v9
      ADD CONSTRAINT engine_runs_v9_decision_fk
      FOREIGN KEY (decision_id)
      REFERENCES public.decision_ledger_v9(decision_id)
      ON DELETE SET NULL;
  END IF;
END$$;

COMMIT;