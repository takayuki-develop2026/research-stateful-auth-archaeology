-- migrations/0020_v18_artifact_assets.sql
-- v18: Artifact registry (derived outputs metadata) - JSONゼロ（本文/実体は外部/S3等。DBはメタのみ）
-- Depends: projects(id varchar(26)), set_updated_at() trigger function (if exists)

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- =========================================================
-- artifact_assets
-- =========================================================
CREATE TABLE IF NOT EXISTS public.artifact_assets (
  id               bigserial PRIMARY KEY,

  project_id        varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,
  artifact_ref      uuid NOT NULL DEFAULT gen_random_uuid(),

  artifact_type     varchar(32) NOT NULL,   -- extracted_text|structured_json|embedding|thumbnail|transcript|features
  schema_version    varchar(64) NOT NULL,   -- e.g. extract.v1 / vision_extract.v1

  -- sha256 is recommended but can be NULL for some generated outputs (still allowed)
  content_sha256    varchar(64) NULL,       -- 64 hex
  content_length    bigint NOT NULL,        -- bytes
  mime_type         varchar(128) NOT NULL,  -- e.g. application/json, text/plain, image/png

  status            varchar(16) NOT NULL DEFAULT 'active', -- active|orphaned|blocked

  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);

-- =========================================================
-- Constraints (domain checks)
-- =========================================================
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='artifact_assets_project_id_nonempty') THEN
    ALTER TABLE public.artifact_assets
      ADD CONSTRAINT artifact_assets_project_id_nonempty CHECK (btrim(project_id::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='artifact_assets_schema_nonempty') THEN
    ALTER TABLE public.artifact_assets
      ADD CONSTRAINT artifact_assets_schema_nonempty CHECK (btrim(schema_version::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='artifact_assets_mime_nonempty') THEN
    ALTER TABLE public.artifact_assets
      ADD CONSTRAINT artifact_assets_mime_nonempty CHECK (btrim(mime_type::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='artifact_assets_content_length_nonneg') THEN
    ALTER TABLE public.artifact_assets
      ADD CONSTRAINT artifact_assets_content_length_nonneg CHECK (content_length >= 0);
  END IF;

  -- sha256 length only when present
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='artifact_assets_sha256_len') THEN
    ALTER TABLE public.artifact_assets
      ADD CONSTRAINT artifact_assets_sha256_len CHECK (content_sha256 IS NULL OR length(content_sha256::text) = 64);
  END IF;

  -- enums via CHECK
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='artifact_assets_artifact_type_ck') THEN
    ALTER TABLE public.artifact_assets
      ADD CONSTRAINT artifact_assets_artifact_type_ck
      CHECK (artifact_type::text = ANY (ARRAY[
        'extracted_text',
        'structured_json',
        'embedding',
        'thumbnail',
        'transcript',
        'features'
      ]::text[]));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='artifact_assets_status_ck') THEN
    ALTER TABLE public.artifact_assets
      ADD CONSTRAINT artifact_assets_status_ck
      CHECK (status::text = ANY (ARRAY[
        'active','orphaned','blocked'
      ]::text[]));
  END IF;
END$$;

-- =========================================================
-- Uniques / Indexes
-- =========================================================
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_artifact_assets_project_ref') THEN
    ALTER TABLE public.artifact_assets
      ADD CONSTRAINT ux_artifact_assets_project_ref UNIQUE (project_id, artifact_ref);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_artifact_assets_project_type_time
  ON public.artifact_assets(project_id, artifact_type, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_artifact_assets_project_status_time
  ON public.artifact_assets(project_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_artifact_assets_project_sha256
  ON public.artifact_assets(project_id, content_sha256);

-- =========================================================
-- updated_at trigger (if set_updated_at() exists)
-- =========================================================
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname='set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_artifact_assets_updated_at') THEN
      CREATE TRIGGER trg_artifact_assets_updated_at
      BEFORE UPDATE ON public.artifact_assets
      FOR EACH ROW
      EXECUTE FUNCTION set_updated_at();
    END IF;
  END IF;
END$$;

COMMIT;