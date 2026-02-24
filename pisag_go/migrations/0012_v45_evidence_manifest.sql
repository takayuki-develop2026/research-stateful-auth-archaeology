-- migrations/0012_v45_evidence_manifest.sql
-- v4.5: Evidence Manifest / Links SoT
--
-- 목적:
-- - v4.4 の run_evidence_assets（body等の実体SoT）に対し、
--   「このrunでどの証拠を保存したか」を宣言・固定する manifest を導入する
-- - worker は fetch結果（body/headers/meta）を assets として保存し、
--   それらを links として束ねた manifest を complete にする
--
-- 方針:
-- - run_evidence_manifests: run_id に対して 0/1（当面）。将来は version を足して複数化可能
-- - run_evidence_links: manifest に紐づく参照（kind + asset_sha256 で冪等）
-- - manifest_hash: links の確定状態をハッシュ化して監査・再現性を担保
--
-- 依存:
-- - public.runs(run_id uuid, trace_id uuid)
-- - public.run_evidence_assets(run_id uuid, trace_id uuid, kind text, content_type text?, byte_size int, sha256 text, final_url text, stored_path text)

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ------------------------------------------
-- 1) run_evidence_manifests
-- ------------------------------------------
CREATE TABLE IF NOT EXISTS public.run_evidence_manifests (
  manifest_id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),

  run_id        uuid NOT NULL REFERENCES public.runs(run_id) ON DELETE CASCADE,
  trace_id      uuid NOT NULL,

  -- building: links are being assembled
  -- complete: manifest_hash fixed, should be treated as SoT for evidence set
  status        text NOT NULL DEFAULT 'building',

  -- sha256 hex (64 chars). NULL while building, filled when complete.
  manifest_hash text NULL,

  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

-- 1 run -> 0/1 manifest (現時点)
CREATE UNIQUE INDEX IF NOT EXISTS run_evidence_manifests_run_uniq
  ON public.run_evidence_manifests (run_id);

CREATE INDEX IF NOT EXISTS run_evidence_manifests_trace_idx
  ON public.run_evidence_manifests (trace_id, created_at);

-- updated_at trigger（runsで使っている set_updated_at があればそれを使う）
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger WHERE tgname = 'trg_run_evidence_manifests_updated_at'
  ) THEN
    -- set_updated_at() が無い環境でも落ちないように定義
    IF NOT EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'set_updated_at') THEN
      CREATE OR REPLACE FUNCTION public.set_updated_at() RETURNS trigger AS $fn$
      BEGIN
        NEW.updated_at := now();
        RETURN NEW;
      END;
      $fn$ LANGUAGE plpgsql;
    END IF;

    CREATE TRIGGER trg_run_evidence_manifests_updated_at
    BEFORE UPDATE ON public.run_evidence_manifests
    FOR EACH ROW
    EXECUTE FUNCTION public.set_updated_at();
  END IF;
END$$;

-- ------------------------------------------
-- 2) run_evidence_links
-- ------------------------------------------
CREATE TABLE IF NOT EXISTS public.run_evidence_links (
  id            bigserial PRIMARY KEY,

  manifest_id   uuid NOT NULL REFERENCES public.run_evidence_manifests(manifest_id) ON DELETE CASCADE,

  -- kind: "fetch_body" / "fetch_headers" / "fetch_meta" etc
  kind          text NOT NULL,

  -- reference to run_evidence_assets.sha256 (same run_id implied by manifest.run_id)
  asset_sha256  text NOT NULL,

  -- duplicated fields for audit convenience (copied from assets at build time)
  content_type  text NULL,
  byte_size     int  NOT NULL,
  final_url     text NOT NULL,
  stored_path   text NOT NULL,

  created_at    timestamptz NOT NULL DEFAULT now()
);

-- Idempotency: same manifest + same kind + same asset
CREATE UNIQUE INDEX IF NOT EXISTS run_evidence_links_uniq
  ON public.run_evidence_links (manifest_id, kind, asset_sha256);

CREATE INDEX IF NOT EXISTS run_evidence_links_manifest_idx
  ON public.run_evidence_links (manifest_id, created_at);

-- Optional: query by kind
CREATE INDEX IF NOT EXISTS run_evidence_links_kind_idx
  ON public.run_evidence_links (kind, created_at);

-- ------------------------------------------
-- 3) Constraints / sanity (soft)
-- ------------------------------------------
-- status enum-like constraint（文字列だが値を絞る）
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'run_evidence_manifests_status_chk'
  ) THEN
    ALTER TABLE public.run_evidence_manifests
      ADD CONSTRAINT run_evidence_manifests_status_chk
      CHECK (status IN ('building','complete'));
  END IF;
END$$;

-- manifest_hash length check (sha256 hex)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'run_evidence_manifests_hash_chk'
  ) THEN
    ALTER TABLE public.run_evidence_manifests
      ADD CONSTRAINT run_evidence_manifests_hash_chk
      CHECK (manifest_hash IS NULL OR manifest_hash ~ '^[0-9a-f]{64}$');
  END IF;
END$$;

COMMIT;