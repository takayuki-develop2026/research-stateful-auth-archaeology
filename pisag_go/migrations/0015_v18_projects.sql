-- migrations/0015_v18_projects.sql
-- v18: Context Expansion (minimal) - projects registry
-- owner=ak で実行する想定

BEGIN;

CREATE TABLE IF NOT EXISTS public.projects (
  project_id  text PRIMARY KEY,
  status      text NOT NULL DEFAULT 'active', -- active|disabled|archived
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

-- optional: status constraint (keep flexible, but prevent empty)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'projects_status_nonempty'
  ) THEN
    ALTER TABLE public.projects
      ADD CONSTRAINT projects_status_nonempty CHECK (btrim(status) <> '');
  END IF;
END$$;

-- updated_at trigger (reuse existing set_updated_at if present)
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_projects_updated_at') THEN
      CREATE TRIGGER trg_projects_updated_at
      BEFORE UPDATE ON public.projects
      FOR EACH ROW
      EXECUTE FUNCTION public.set_updated_at();
    END IF;
  ELSE
    -- fallback: define trigger function locally if not present
    CREATE OR REPLACE FUNCTION public.set_updated_at()
    RETURNS trigger
    LANGUAGE plpgsql
    AS $fn$
    BEGIN
      NEW.updated_at := now();
      RETURN NEW;
    END;
    $fn$;

    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_projects_updated_at') THEN
      CREATE TRIGGER trg_projects_updated_at
      BEFORE UPDATE ON public.projects
      FOR EACH ROW
      EXECUTE FUNCTION public.set_updated_at();
    END IF;
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_projects_status_created
  ON public.projects(status, created_at DESC);

COMMIT;