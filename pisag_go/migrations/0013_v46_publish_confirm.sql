-- migrations/0013_v46_publish_confirm.sql
-- v4.6: Publish-Confirm SoT (idempotent commit)
--
-- 목적:
-- - v4.5 の evidence manifest（run_evidence_manifests + links）を入力に、
--   「公開（publish）確定」を SoT として記録する
-- - idempotency は (project_id, commit_key) で担保（run_id を含めない）
--
-- 方針:
-- - catalog_publish_commits が SoT
-- - commit_key は sha256 hex（64 chars）を推奨（生成はアプリ側）
-- - approval は v4.7 で導入するため、v4.6 では status だけ持つ（default: proposed）
--
-- 依存:
-- - public.runs(run_id uuid, project_id text, trace_id uuid)
-- - public.run_evidence_manifests(manifest_id uuid, run_id uuid, trace_id uuid, status, manifest_hash)

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ------------------------------------------
-- 1) catalog_publish_commits (SoT)
-- ------------------------------------------
CREATE TABLE IF NOT EXISTS public.catalog_publish_commits (
  commit_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),

  -- tenant/project scope
  project_id     text NOT NULL,

  -- idempotency key (must NOT include run_id)
  -- e.g. sha256("publish|project|target|manifest_hash|policy_version|...") hex(64)
  commit_key     text NOT NULL,

  -- evidence input
  manifest_id    uuid NOT NULL REFERENCES public.run_evidence_manifests(manifest_id) ON DELETE RESTRICT,
  manifest_hash  text NOT NULL,

  -- traceability (optional but recommended)
  run_id         uuid NULL REFERENCES public.runs(run_id) ON DELETE SET NULL,
  trace_id       uuid NOT NULL,

  -- publish target (optional: which catalog/table/endpoint)
  target         text NOT NULL DEFAULT 'catalog_v1',

  -- status:
  -- proposed  : created, waiting for approval (v4.7) or auto-confirm path
  -- confirmed : publish confirmed (SoT)
  -- failed    : publish failed (error_code/message set)
  status        text NOT NULL DEFAULT 'proposed',

  error_code    text NULL,
  error_message text NULL,

  -- optional metadata for audit (non-SoT payload snapshot)
  meta_json     jsonb NOT NULL DEFAULT '{}'::jsonb,

  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Idempotency (global within project)
CREATE UNIQUE INDEX IF NOT EXISTS catalog_publish_commits_project_key_uniq
  ON public.catalog_publish_commits (project_id, commit_key);

-- Query helpers
CREATE INDEX IF NOT EXISTS catalog_publish_commits_project_created_idx
  ON public.catalog_publish_commits (project_id, created_at DESC);

CREATE INDEX IF NOT EXISTS catalog_publish_commits_manifest_idx
  ON public.catalog_publish_commits (manifest_id, created_at DESC);

CREATE INDEX IF NOT EXISTS catalog_publish_commits_trace_idx
  ON public.catalog_publish_commits (trace_id, created_at DESC);

-- status constraint
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'catalog_publish_commits_status_chk'
  ) THEN
    ALTER TABLE public.catalog_publish_commits
      ADD CONSTRAINT catalog_publish_commits_status_chk
      CHECK (status IN ('proposed','confirmed','failed'));
  END IF;
END$$;

-- manifest_hash constraint (sha256 hex)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'catalog_publish_commits_manifest_hash_chk'
  ) THEN
    ALTER TABLE public.catalog_publish_commits
      ADD CONSTRAINT catalog_publish_commits_manifest_hash_chk
      CHECK (manifest_hash ~ '^[0-9a-f]{64}$');
  END IF;
END$$;

-- commit_key non-empty (and optionally sha256 hex)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'catalog_publish_commits_commit_key_chk'
  ) THEN
    ALTER TABLE public.catalog_publish_commits
      ADD CONSTRAINT catalog_publish_commits_commit_key_chk
      CHECK (btrim(commit_key) <> '');
  END IF;
END$$;

-- ------------------------------------------
-- 2) updated_at trigger (reuse set_updated_at if exists)
-- ------------------------------------------
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger WHERE tgname = 'trg_catalog_publish_commits_updated_at'
  ) THEN
    IF NOT EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'set_updated_at') THEN
      CREATE OR REPLACE FUNCTION public.set_updated_at() RETURNS trigger AS $fn$
      BEGIN
        NEW.updated_at := now();
        RETURN NEW;
      END;
      $fn$ LANGUAGE plpgsql;
    END IF;

    CREATE TRIGGER trg_catalog_publish_commits_updated_at
    BEFORE UPDATE ON public.catalog_publish_commits
    FOR EACH ROW
    EXECUTE FUNCTION public.set_updated_at();
  END IF;
END$$;

COMMIT;