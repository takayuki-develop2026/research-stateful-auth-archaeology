-- migrations/0021_v18_artifact_evidence_links.sql
-- v18: artifact ⇄ evidence 多対多（JSONゼロ）
-- Depends: projects, evidence_assets, artifact_assets, set_updated_at() (optional)

BEGIN;

-- =========================================================
-- artifact_evidence_links
-- =========================================================
CREATE TABLE IF NOT EXISTS public.artifact_evidence_links (
  id            bigserial PRIMARY KEY,

  project_id     varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  artifact_ref   uuid NOT NULL,
  evidence_ref   uuid NOT NULL,

  link_role      varchar(16) NOT NULL, -- input|intermediate|supporting|output_proof

  created_at     timestamptz NOT NULL DEFAULT now()
);

-- FK-ish: validate refs within same project using composite unique constraints
-- (project_id, artifact_ref) and (project_id, evidence_ref) are unique in their tables.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_artifact_evidence_links_artifact_ref') THEN
    ALTER TABLE public.artifact_evidence_links
      ADD CONSTRAINT fk_artifact_evidence_links_artifact_ref
      FOREIGN KEY (project_id, artifact_ref)
      REFERENCES public.artifact_assets(project_id, artifact_ref)
      ON DELETE CASCADE;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_artifact_evidence_links_evidence_ref') THEN
    ALTER TABLE public.artifact_evidence_links
      ADD CONSTRAINT fk_artifact_evidence_links_evidence_ref
      FOREIGN KEY (project_id, evidence_ref)
      REFERENCES public.evidence_assets(project_id, evidence_ref)
      ON DELETE CASCADE;
  END IF;
END$$;

-- =========================================================
-- Constraints
-- =========================================================
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='artifact_evidence_links_project_id_nonempty') THEN
    ALTER TABLE public.artifact_evidence_links
      ADD CONSTRAINT artifact_evidence_links_project_id_nonempty CHECK (btrim(project_id::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='artifact_evidence_links_link_role_ck') THEN
    ALTER TABLE public.artifact_evidence_links
      ADD CONSTRAINT artifact_evidence_links_link_role_ck
      CHECK (link_role::text = ANY (ARRAY[
        'input','intermediate','supporting','output_proof'
      ]::text[]));
  END IF;
END$$;

-- =========================================================
-- Uniques / Indexes
-- =========================================================
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_artifact_evidence_links_unique') THEN
    ALTER TABLE public.artifact_evidence_links
      ADD CONSTRAINT ux_artifact_evidence_links_unique
      UNIQUE (project_id, artifact_ref, evidence_ref, link_role);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_artifact_evidence_links_project_evidence
  ON public.artifact_evidence_links(project_id, evidence_ref);

CREATE INDEX IF NOT EXISTS idx_artifact_evidence_links_project_artifact
  ON public.artifact_evidence_links(project_id, artifact_ref);

CREATE INDEX IF NOT EXISTS idx_artifact_evidence_links_project_role_time
  ON public.artifact_evidence_links(project_id, link_role, created_at DESC);

COMMIT;