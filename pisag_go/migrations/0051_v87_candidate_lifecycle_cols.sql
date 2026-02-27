-- migrations/0051_v87_candidate_lifecycle_cols.sql
-- v8.7: lifecycle columns on discovery_candidates (non-breaking ALTER)

BEGIN;

-- Add columns only if missing (safe re-run style)
DO $$
BEGIN
  -- stale/expire/archive
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='stale_at') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN stale_at timestamptz NULL;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='expires_at') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN expires_at timestamptz NULL;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='archived_at') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN archived_at timestamptz NULL;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='archive_reason') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN archive_reason varchar(32) NULL;
  END IF;

  -- retry (processing failures)
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='retry_attempts') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN retry_attempts int NOT NULL DEFAULT 0;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='retry_next_at') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN retry_next_at timestamptz NULL;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='retry_last_code') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN retry_last_code varchar(64) NULL;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='retry_last_message') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN retry_last_message varchar(256) NULL;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='retry_last_evidence_ref') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN retry_last_evidence_ref uuid NULL;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='retry_backoff_sec') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN retry_backoff_sec int NOT NULL DEFAULT 0;
  END IF;

  -- apply retry (publish/confirm failures)
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='apply_attempts') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN apply_attempts int NOT NULL DEFAULT 0;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='apply_next_at') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN apply_next_at timestamptz NULL;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='apply_last_code') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN apply_last_code varchar(64) NULL;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='apply_last_message') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN apply_last_message varchar(256) NULL;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='apply_last_evidence_ref') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN apply_last_evidence_ref uuid NULL;
  END IF;

  -- lifecycle version
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='discovery_candidates' AND column_name='lifecycle_version') THEN
    ALTER TABLE public.discovery_candidates ADD COLUMN lifecycle_version varchar(16) NOT NULL DEFAULT 'lc_v1';
  END IF;
END$$;

-- Indexes
CREATE INDEX IF NOT EXISTS idx_candidates_v87_project_stale
  ON public.discovery_candidates(project_id, stale_at) WHERE stale_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_candidates_v87_project_archived
  ON public.discovery_candidates(project_id, archived_at) WHERE archived_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_candidates_v87_project_retry_due
  ON public.discovery_candidates(project_id, retry_next_at) WHERE retry_next_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_candidates_v87_project_apply_retry_due
  ON public.discovery_candidates(project_id, apply_next_at) WHERE apply_next_at IS NOT NULL;

COMMIT;