-- migrations/0060_v90_engine_runs.sql
-- v9.0: Engine Router - engine_runs_v9 (execution ledger)
-- Depends:
-- - public.projects(id varchar(26))
-- - public.runs(run_id uuid, project_id)
-- - public.evidence_assets(id bigint)
-- - set_updated_at() trigger function (if exists)
BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.engine_runs_v9 (
  engine_run_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),

  project_id         varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  run_id             uuid NOT NULL REFERENCES public.runs(run_id) ON DELETE RESTRICT,
  trace_id           uuid NOT NULL,

  task_type          varchar(64) NOT NULL,
  mode               varchar(24) NOT NULL, -- mode0_rule_only|mode1_rule_plus|mode2_llm_primary|mode3_human_only|mode4_fallback_only

  pipeline_version   varchar(32) NOT NULL,
  policy_version     varchar(32) NOT NULL,

  principal_hash     char(64) NOT NULL,
  input_hash         char(64) NOT NULL,

  status             varchar(24) NOT NULL DEFAULT 'queued', -- queued|running|succeeded|review_required|failed_recorded|skipped

  decision_id        uuid NULL, -- FK added in 0061 (after decision_ledger exists)
  proposal_ref       uuid NULL, -- optional (future v8.7 proposal ref or evidence ref)

  idempotency_key    text NOT NULL, -- scope included (v13)
  cache_key          char(64) NULL,

  started_at         timestamptz NULL,
  finished_at        timestamptz NULL,

  error_type         varchar(64) NULL,
  error_summary      varchar(256) NULL,
  error_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='engine_runs_v9_task_nonempty') THEN
    ALTER TABLE public.engine_runs_v9
      ADD CONSTRAINT engine_runs_v9_task_nonempty CHECK (btrim(task_type::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='engine_runs_v9_mode_nonempty') THEN
    ALTER TABLE public.engine_runs_v9
      ADD CONSTRAINT engine_runs_v9_mode_nonempty CHECK (btrim(mode::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='engine_runs_v9_versions_nonempty') THEN
    ALTER TABLE public.engine_runs_v9
      ADD CONSTRAINT engine_runs_v9_versions_nonempty CHECK (btrim(pipeline_version::text) <> '' AND btrim(policy_version::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='engine_runs_v9_hash_len') THEN
    ALTER TABLE public.engine_runs_v9
      ADD CONSTRAINT engine_runs_v9_hash_len CHECK (length(principal_hash) = 64 AND length(input_hash) = 64);
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='engine_runs_v9_status_ck') THEN
    ALTER TABLE public.engine_runs_v9
      ADD CONSTRAINT engine_runs_v9_status_ck CHECK (lower(status::text) IN (
        'queued','running','succeeded','review_required','failed_recorded','skipped'
      ));
  END IF;

  -- Stable de-dupe key (cache stability + v13 idempotency double guard)
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_engine_runs_v9_stable') THEN
    ALTER TABLE public.engine_runs_v9
      ADD CONSTRAINT ux_engine_runs_v9_stable UNIQUE (
        project_id, task_type, mode, pipeline_version, policy_version, principal_hash, input_hash
      );
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_engine_runs_v9_project_status_time
  ON public.engine_runs_v9(project_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_engine_runs_v9_project_task_time
  ON public.engine_runs_v9(project_id, task_type, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_engine_runs_v9_project_run
  ON public.engine_runs_v9(project_id, run_id);

CREATE INDEX IF NOT EXISTS idx_engine_runs_v9_project_cache
  ON public.engine_runs_v9(project_id, cache_key);

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname='set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_engine_runs_v9_updated_at') THEN
      CREATE TRIGGER trg_engine_runs_v9_updated_at
      BEFORE UPDATE ON public.engine_runs_v9
      FOR EACH ROW
      EXECUTE FUNCTION set_updated_at();
    END IF;
  END IF;
END$$;

COMMIT;