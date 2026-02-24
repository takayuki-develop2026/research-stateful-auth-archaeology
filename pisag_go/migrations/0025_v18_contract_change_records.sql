-- migrations/0025_v18_contract_change_records.sql
-- v18: 契約変更の固定（監査と再現）
-- 内容本文は evidence_assets を参照（before/after の contract_evidence_ref を持つ）
--
-- Depends:
-- - projects(id varchar(26))
-- - evidence_assets(project_id, evidence_ref) unique
-- - runs(run_id uuid) は任意（system更新runがある場合だけ run_id を入れる）

BEGIN;

CREATE TABLE IF NOT EXISTS public.contract_change_records (
  id                         bigserial PRIMARY KEY,

  project_id                 varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  task_type                  varchar(32) NOT NULL,
  pipeline_version           varchar(64) NOT NULL,

  change_kind                varchar(16) NOT NULL, -- created|updated|enabled|disabled

  before_contract_evidence_ref uuid NULL,
  after_contract_evidence_ref  uuid NULL,

  trace_id                   varchar(64) NOT NULL,
  run_id                     uuid NULL,

  created_by_user_id         varchar(128) NULL,

  created_at                 timestamptz NOT NULL DEFAULT now()
);

-- Optional FK-ish (evidence refs must belong to same project if present)
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_contract_change_records_before_evidence') THEN
    ALTER TABLE public.contract_change_records
      ADD CONSTRAINT fk_contract_change_records_before_evidence
      FOREIGN KEY (project_id, before_contract_evidence_ref)
      REFERENCES public.evidence_assets(project_id, evidence_ref)
      ON DELETE RESTRICT;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_contract_change_records_after_evidence') THEN
    ALTER TABLE public.contract_change_records
      ADD CONSTRAINT fk_contract_change_records_after_evidence
      FOREIGN KEY (project_id, after_contract_evidence_ref)
      REFERENCES public.evidence_assets(project_id, evidence_ref)
      ON DELETE RESTRICT;
  END IF;
END$$;

-- Optional FK to runs (only if you want strict referential integrity)
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_contract_change_records_run_id') THEN
    -- only add FK if runs table exists
    IF EXISTS (SELECT 1 FROM pg_class WHERE relname='runs' AND relkind='r') THEN
      ALTER TABLE public.contract_change_records
        ADD CONSTRAINT fk_contract_change_records_run_id
        FOREIGN KEY (run_id)
        REFERENCES public.runs(run_id)
        ON DELETE SET NULL;
    END IF;
  END IF;
END$$;

-- Constraints
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='contract_change_records_project_id_nonempty') THEN
    ALTER TABLE public.contract_change_records
      ADD CONSTRAINT contract_change_records_project_id_nonempty CHECK (btrim(project_id::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='contract_change_records_task_type_nonempty') THEN
    ALTER TABLE public.contract_change_records
      ADD CONSTRAINT contract_change_records_task_type_nonempty CHECK (btrim(task_type::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='contract_change_records_pipeline_version_nonempty') THEN
    ALTER TABLE public.contract_change_records
      ADD CONSTRAINT contract_change_records_pipeline_version_nonempty CHECK (btrim(pipeline_version::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='contract_change_records_trace_nonempty') THEN
    ALTER TABLE public.contract_change_records
      ADD CONSTRAINT contract_change_records_trace_nonempty CHECK (btrim(trace_id::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='contract_change_records_change_kind_ck') THEN
    ALTER TABLE public.contract_change_records
      ADD CONSTRAINT contract_change_records_change_kind_ck
      CHECK (change_kind::text = ANY (ARRAY[
        'created','updated','enabled','disabled'
      ]::text[]));
  END IF;

  -- require at least one of before/after to be present
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='contract_change_records_before_after_present') THEN
    ALTER TABLE public.contract_change_records
      ADD CONSTRAINT contract_change_records_before_after_present
      CHECK (
        before_contract_evidence_ref IS NOT NULL
        OR after_contract_evidence_ref IS NOT NULL
      );
  END IF;
END$$;

-- Indexes
CREATE INDEX IF NOT EXISTS idx_contract_change_records_project_task_time
  ON public.contract_change_records(project_id, task_type, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_contract_change_records_project_trace_time
  ON public.contract_change_records(project_id, trace_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_contract_change_records_project_time
  ON public.contract_change_records(project_id, created_at DESC);

COMMIT;