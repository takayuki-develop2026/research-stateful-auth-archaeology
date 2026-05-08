-- migrations/0023_v18_run_artifact_links.sql
-- v18: run ⇄ artifact links（このrunが生成した成果物を固定）
-- JSONゼロ。多対多はリンクテーブルで表現。
-- Depends:
-- - projects(id varchar(26))
-- - runs(run_id uuid)
-- - artifact_assets(project_id, artifact_ref) unique

BEGIN;

-- =========================================================
-- run_artifact_links
-- =========================================================
CREATE TABLE IF NOT EXISTS public.run_artifact_links (
  id          bigserial PRIMARY KEY,

  project_id   varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  run_id       uuid NOT NULL REFERENCES public.runs(run_id) ON DELETE CASCADE,

  artifact_ref uuid NOT NULL,

  role        varchar(24) NOT NULL, -- primary_output|secondary_output|debug_output

  created_at  timestamptz NOT NULL DEFAULT now()
);

-- FK-ish: ensure artifact_ref belongs to same project_id
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_run_artifact_links_artifact_ref') THEN
    ALTER TABLE public.run_artifact_links
      ADD CONSTRAINT fk_run_artifact_links_artifact_ref
      FOREIGN KEY (project_id, artifact_ref)
      REFERENCES public.artifact_assets(project_id, artifact_ref)
      ON DELETE CASCADE;
  END IF;
END$$;

-- =========================================================
-- Constraints
-- =========================================================
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='run_artifact_links_project_id_nonempty') THEN
    ALTER TABLE public.run_artifact_links
      ADD CONSTRAINT run_artifact_links_project_id_nonempty CHECK (btrim(project_id::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='run_artifact_links_role_ck') THEN
    ALTER TABLE public.run_artifact_links
      ADD CONSTRAINT run_artifact_links_role_ck
      CHECK (role::text = ANY (ARRAY[
        'primary_output','secondary_output','debug_output'
      ]::text[]));
  END IF;
END$$;

-- =========================================================
-- Uniques / Indexes
-- =========================================================
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_run_artifact_links_unique') THEN
    ALTER TABLE public.run_artifact_links
      ADD CONSTRAINT ux_run_artifact_links_unique
      UNIQUE (project_id, run_id, artifact_ref, role);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_run_artifact_links_project_artifact
  ON public.run_artifact_links(project_id, artifact_ref);

CREATE INDEX IF NOT EXISTS idx_run_artifact_links_project_run
  ON public.run_artifact_links(project_id, run_id);

CREATE INDEX IF NOT EXISTS idx_run_artifact_links_project_role_time
  ON public.run_artifact_links(project_id, role, created_at DESC);

COMMIT;