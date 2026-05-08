-- migrations/0062_v90_engine_cache.sql
-- v9.0: engine_cache_v9
-- Depends: engine_runs_v9, decision_ledger_v9, projects
BEGIN;

CREATE TABLE IF NOT EXISTS public.engine_cache_v9 (
  id            bigserial PRIMARY KEY,

  project_id     varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  cache_key      char(64) NOT NULL,

  engine_run_id  uuid NOT NULL REFERENCES public.engine_runs_v9(engine_run_id) ON DELETE CASCADE,
  decision_id    uuid NOT NULL REFERENCES public.decision_ledger_v9(decision_id) ON DELETE CASCADE,

  expires_at     timestamptz NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='engine_cache_v9_key_len') THEN
    ALTER TABLE public.engine_cache_v9
      ADD CONSTRAINT engine_cache_v9_key_len CHECK (length(cache_key) = 64);
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_engine_cache_v9') THEN
    ALTER TABLE public.engine_cache_v9
      ADD CONSTRAINT ux_engine_cache_v9 UNIQUE (project_id, cache_key);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_engine_cache_v9_project_expires
  ON public.engine_cache_v9(project_id, expires_at);

COMMIT;