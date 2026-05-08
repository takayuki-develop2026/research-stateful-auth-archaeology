-- migrations/0024_v18_task_type_contracts.sql
-- v18: task_type 契約台帳（projectごと / JSONゼロ）
-- 契約本文（schema等）は evidence_assets を参照（input_contract_evidence_ref / output_contract_evidence_ref）
--
-- Depends:
-- - projects(id varchar(26))
-- - evidence_assets(project_id, evidence_ref) unique
-- - (optional) policy_versions / rulesets などが後続で入るなら policy_version_id は varchar(26)のまま保持

BEGIN;

-- =========================================================
-- task_type_contracts
-- =========================================================
CREATE TABLE IF NOT EXISTS public.task_type_contracts (
  id                           bigserial PRIMARY KEY,

  project_id                   varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  task_type                    varchar(32) NOT NULL,   -- fulltext_extract|vision_extract|audio_transcribe|behavior_ingest...
  pipeline_version             varchar(64) NOT NULL,   -- e.g. v18.0 / v4.5 etc
  policy_version_id            varchar(26) NULL,       -- published policy_versions へ将来FK想定（現時点NULL可）

  enabled                      boolean NOT NULL DEFAULT true,

  input_contract_evidence_ref  uuid NOT NULL,
  output_contract_evidence_ref uuid NOT NULL,

  default_mode                 varchar(16) NULL,       -- Mode0..Mode4 を想定（文字列固定）

  created_by_type              varchar(16) NOT NULL DEFAULT 'system', -- system|user|service
  created_by_id                varchar(128) NULL,

  created_at                   timestamptz NOT NULL DEFAULT now(),
  updated_at                   timestamptz NOT NULL DEFAULT now()
);

-- FK-ish: ensure contract evidence refs belong to the same project
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_task_type_contracts_input_contract') THEN
    ALTER TABLE public.task_type_contracts
      ADD CONSTRAINT fk_task_type_contracts_input_contract
      FOREIGN KEY (project_id, input_contract_evidence_ref)
      REFERENCES public.evidence_assets(project_id, evidence_ref)
      ON DELETE RESTRICT;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_task_type_contracts_output_contract') THEN
    ALTER TABLE public.task_type_contracts
      ADD CONSTRAINT fk_task_type_contracts_output_contract
      FOREIGN KEY (project_id, output_contract_evidence_ref)
      REFERENCES public.evidence_assets(project_id, evidence_ref)
      ON DELETE RESTRICT;
  END IF;
END$$;

-- =========================================================
-- Constraints
-- =========================================================
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='task_type_contracts_project_id_nonempty') THEN
    ALTER TABLE public.task_type_contracts
      ADD CONSTRAINT task_type_contracts_project_id_nonempty CHECK (btrim(project_id::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='task_type_contracts_task_type_nonempty') THEN
    ALTER TABLE public.task_type_contracts
      ADD CONSTRAINT task_type_contracts_task_type_nonempty CHECK (btrim(task_type::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='task_type_contracts_pipeline_version_nonempty') THEN
    ALTER TABLE public.task_type_contracts
      ADD CONSTRAINT task_type_contracts_pipeline_version_nonempty CHECK (btrim(pipeline_version::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='task_type_contracts_created_by_type_ck') THEN
    ALTER TABLE public.task_type_contracts
      ADD CONSTRAINT task_type_contracts_created_by_type_ck
      CHECK (created_by_type::text = ANY (ARRAY[
        'system','user','service'
      ]::text[]));
  END IF;

  -- Optional: default_mode enum-like
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='task_type_contracts_default_mode_ck') THEN
    ALTER TABLE public.task_type_contracts
      ADD CONSTRAINT task_type_contracts_default_mode_ck
      CHECK (
        default_mode IS NULL OR
        default_mode::text = ANY (ARRAY['Mode0','Mode1','Mode2','Mode3','Mode4']::text[])
      );
  END IF;
END$$;

-- =========================================================
-- Uniques / Indexes
-- =========================================================
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_task_type_contracts_task_pipeline') THEN
    ALTER TABLE public.task_type_contracts
      ADD CONSTRAINT ux_task_type_contracts_task_pipeline
      UNIQUE (project_id, task_type, pipeline_version);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_task_type_contracts_project_task_enabled
  ON public.task_type_contracts(project_id, task_type, enabled);

CREATE INDEX IF NOT EXISTS idx_task_type_contracts_project_enabled
  ON public.task_type_contracts(project_id, enabled, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_task_type_contracts_project_policy
  ON public.task_type_contracts(project_id, policy_version_id);

-- =========================================================
-- updated_at trigger (if set_updated_at() exists)
-- =========================================================
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname='set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_task_type_contracts_updated_at') THEN
      CREATE TRIGGER trg_task_type_contracts_updated_at
      BEFORE UPDATE ON public.task_type_contracts
      FOR EACH ROW
      EXECUTE FUNCTION set_updated_at();
    END IF;
  END IF;
END$$;

COMMIT;