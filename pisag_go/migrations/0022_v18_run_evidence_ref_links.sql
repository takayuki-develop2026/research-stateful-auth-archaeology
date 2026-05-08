-- migrations/0022_v18_run_evidence_ref_links.sql
-- v18: run ⇄ evidence_ref links（runが何のevidenceを入力/取得/生成したかを固定）
-- IMPORTANT:
-- - DO NOT use "run_evidence_links" table name (already used by v4.5 manifest links).
-- - v18 uses "run_evidence_ref_links".
--
-- Depends:
-- - projects(id varchar(26))
-- - runs(run_id uuid, project_id text/varchar)
-- - evidence_assets(project_id, evidence_ref) unique

BEGIN;

-- =========================================================
-- run_evidence_ref_links
-- =========================================================
CREATE TABLE IF NOT EXISTS public.run_evidence_ref_links (
  id           bigserial PRIMARY KEY,

  project_id   varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  run_id       uuid NOT NULL REFERENCES public.runs(run_id) ON DELETE CASCADE,

  evidence_ref uuid NOT NULL,

  role         varchar(24) NOT NULL, -- input|fetched|uploaded|generated|webhook_source

  created_at   timestamptz NOT NULL DEFAULT now()
);

-- FK: ensure evidence_ref belongs to same project_id
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_run_evidence_ref_links_evidence_ref') THEN
    ALTER TABLE public.run_evidence_ref_links
      ADD CONSTRAINT fk_run_evidence_ref_links_evidence_ref
      FOREIGN KEY (project_id, evidence_ref)
      REFERENCES public.evidence_assets(project_id, evidence_ref)
      ON DELETE RESTRICT;
  END IF;
END$$;

-- =========================================================
-- Constraints
-- =========================================================
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='run_evidence_ref_links_project_id_nonempty') THEN
    ALTER TABLE public.run_evidence_ref_links
      ADD CONSTRAINT run_evidence_ref_links_project_id_nonempty CHECK (btrim(project_id::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='run_evidence_ref_links_role_ck') THEN
    ALTER TABLE public.run_evidence_ref_links
      ADD CONSTRAINT run_evidence_ref_links_role_ck
      CHECK (role::text = ANY (ARRAY[
        'input','fetched','uploaded','generated','webhook_source'
      ]::text[]));
  END IF;
END$$;

-- =========================================================
-- Uniques / Indexes
-- =========================================================
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_run_evidence_ref_links_unique') THEN
    ALTER TABLE public.run_evidence_ref_links
      ADD CONSTRAINT ux_run_evidence_ref_links_unique
      UNIQUE (project_id, run_id, evidence_ref, role);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_run_evidence_ref_links_project_evidence
  ON public.run_evidence_ref_links(project_id, evidence_ref);

CREATE INDEX IF NOT EXISTS idx_run_evidence_ref_links_project_run
  ON public.run_evidence_ref_links(project_id, run_id);

CREATE INDEX IF NOT EXISTS idx_run_evidence_ref_links_project_role_time
  ON public.run_evidence_ref_links(project_id, role, created_at DESC);

-- =========================================================
-- P0 Hardening: prevent cross-project run_id mismatch
-- (project_id on link MUST match runs.project_id)
-- =========================================================
CREATE OR REPLACE FUNCTION public.ensure_run_evidence_ref_links_run_project_match()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  v_run_project text;
BEGIN
  SELECT r.project_id::text
    INTO v_run_project
  FROM public.runs r
  WHERE r.run_id = NEW.run_id;

  IF v_run_project IS NULL THEN
    RAISE EXCEPTION 'run_id not found: %', NEW.run_id USING ERRCODE = '23503';
  END IF;

  IF v_run_project <> NEW.project_id::text THEN
    RAISE EXCEPTION 'run.project_id mismatch: run_id=% project_id(link)=% project_id(run)=%',
      NEW.run_id, NEW.project_id, v_run_project
      USING ERRCODE = '23514';
  END IF;

  RETURN NEW;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_run_evidence_ref_links_run_project_match') THEN
    CREATE TRIGGER trg_run_evidence_ref_links_run_project_match
    BEFORE INSERT OR UPDATE OF project_id, run_id
    ON public.run_evidence_ref_links
    FOR EACH ROW
    EXECUTE FUNCTION public.ensure_run_evidence_ref_links_run_project_match();
  END IF;
END$$;

COMMIT;