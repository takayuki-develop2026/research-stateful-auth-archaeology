-- migrations/0019_v18_evidence_assets.sql
-- v18: Evidence registry (metadata ledger) - JSONゼロ（本文は外部/S3等。DBはメタのみ）
-- Depends: projects(id varchar(26)), set_updated_at() trigger function (if exists)

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- =========================================================
-- evidence_assets
-- =========================================================
CREATE TABLE IF NOT EXISTS public.evidence_assets (
  id               bigserial PRIMARY KEY,

  project_id        varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  evidence_ref      uuid NOT NULL DEFAULT gen_random_uuid(),

  media_type        varchar(16) NOT NULL,  -- text|image|audio|video|binary
  source_kind       varchar(24) NOT NULL,  -- pisag_fetch|upload|webhook|generated|import
  source_uri        text NULL,             -- 補助情報（URL等）。同一性の根拠にはしない

  content_sha256    varchar(64) NOT NULL,  -- 64 hex
  content_length    bigint NOT NULL,       -- bytes
  mime_type         varchar(128) NOT NULL, -- e.g. image/png, application/json
  language          varchar(16) NULL,      -- e.g. ja, en

  retention_policy  varchar(16) NOT NULL DEFAULT 'standard', -- short|standard|legal_hold
  expires_at_utc    timestamptz NULL,
  status            varchar(16) NOT NULL DEFAULT 'active',   -- active|expired|tombstoned

  created_by_type   varchar(16) NOT NULL,  -- system|user|service
  created_by_id     varchar(128) NULL,

  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);

-- =========================================================
-- Constraints (domain checks)
-- =========================================================
DO $$
BEGIN
  -- non-empty checks
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='evidence_assets_project_id_nonempty') THEN
    ALTER TABLE public.evidence_assets
      ADD CONSTRAINT evidence_assets_project_id_nonempty CHECK (btrim(project_id::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='evidence_assets_mime_nonempty') THEN
    ALTER TABLE public.evidence_assets
      ADD CONSTRAINT evidence_assets_mime_nonempty CHECK (btrim(mime_type::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='evidence_assets_sha256_len') THEN
    ALTER TABLE public.evidence_assets
      ADD CONSTRAINT evidence_assets_sha256_len CHECK (length(content_sha256::text) = 64);
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='evidence_assets_content_length_nonneg') THEN
    ALTER TABLE public.evidence_assets
      ADD CONSTRAINT evidence_assets_content_length_nonneg CHECK (content_length >= 0);
  END IF;

  -- enums via CHECK
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='evidence_assets_media_type_ck') THEN
    ALTER TABLE public.evidence_assets
      ADD CONSTRAINT evidence_assets_media_type_ck
      CHECK (media_type::text = ANY (ARRAY[
        'text','image','audio','video','binary'
      ]::text[]));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='evidence_assets_source_kind_ck') THEN
    ALTER TABLE public.evidence_assets
      ADD CONSTRAINT evidence_assets_source_kind_ck
      CHECK (source_kind::text = ANY (ARRAY[
        'pisag_fetch','upload','webhook','generated','import'
      ]::text[]));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='evidence_assets_retention_policy_ck') THEN
    ALTER TABLE public.evidence_assets
      ADD CONSTRAINT evidence_assets_retention_policy_ck
      CHECK (retention_policy::text = ANY (ARRAY[
        'short','standard','legal_hold'
      ]::text[]));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='evidence_assets_status_ck') THEN
    ALTER TABLE public.evidence_assets
      ADD CONSTRAINT evidence_assets_status_ck
      CHECK (status::text = ANY (ARRAY[
        'active','expired','tombstoned'
      ]::text[]));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='evidence_assets_created_by_type_ck') THEN
    ALTER TABLE public.evidence_assets
      ADD CONSTRAINT evidence_assets_created_by_type_ck
      CHECK (created_by_type::text = ANY (ARRAY[
        'system','user','service'
      ]::text[]));
  END IF;
END$$;

-- =========================================================
-- Uniques / Indexes
-- =========================================================
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname='ux_evidence_assets_project_ref'
  ) THEN
    ALTER TABLE public.evidence_assets
      ADD CONSTRAINT ux_evidence_assets_project_ref UNIQUE (project_id, evidence_ref);
  END IF;
END$$;

-- 推奨（ただし運用方針でON/OFF）：project内sha256重複排除をしたいなら有効化
-- DO $$ BEGIN
--   IF NOT EXISTS (
--     SELECT 1 FROM pg_constraint
--     WHERE conname='ux_evidence_assets_project_sha256'
--   ) THEN
--     ALTER TABLE public.evidence_assets
--       ADD CONSTRAINT ux_evidence_assets_project_sha256 UNIQUE (project_id, content_sha256);
--   END IF;
-- END$$;

CREATE INDEX IF NOT EXISTS idx_evidence_assets_project_media_time
  ON public.evidence_assets(project_id, media_type, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_evidence_assets_project_status_expires
  ON public.evidence_assets(project_id, status, expires_at_utc);

CREATE INDEX IF NOT EXISTS idx_evidence_assets_project_sha256
  ON public.evidence_assets(project_id, content_sha256);

CREATE INDEX IF NOT EXISTS idx_evidence_assets_project_created
  ON public.evidence_assets(project_id, created_at DESC);

-- =========================================================
-- updated_at trigger (if set_updated_at() exists)
-- =========================================================
DO $$
BEGIN
  -- set_updated_at() の存在チェック
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname='set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_evidence_assets_updated_at') THEN
      CREATE TRIGGER trg_evidence_assets_updated_at
      BEFORE UPDATE ON public.evidence_assets
      FOR EACH ROW
      EXECUTE FUNCTION set_updated_at();
    END IF;
  END IF;
END$$;

COMMIT;