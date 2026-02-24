-- migrations/0003_v43_run_evidence_assets.sql (revised, for fresh DB)
-- 목적: evidence assets SoT（v4.3）
-- 方針: run_id uuid FK, trace_id uuid

BEGIN;

CREATE TABLE IF NOT EXISTS public.run_evidence_assets (
  id           bigserial PRIMARY KEY,
  run_id       uuid NOT NULL REFERENCES public.runs(run_id) ON DELETE CASCADE,
  trace_id     uuid NOT NULL,
  kind         text NOT NULL,
  content_type text NULL,
  byte_size    int  NOT NULL,
  sha256       text NOT NULL,
  final_url    text NOT NULL,
  stored_path  text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS run_evidence_assets_uniq
  ON public.run_evidence_assets (run_id, kind, sha256);

CREATE INDEX IF NOT EXISTS run_evidence_assets_run_idx
  ON public.run_evidence_assets (run_id, created_at);

CREATE INDEX IF NOT EXISTS run_evidence_assets_trace_idx
  ON public.run_evidence_assets (trace_id, created_at);

COMMIT;