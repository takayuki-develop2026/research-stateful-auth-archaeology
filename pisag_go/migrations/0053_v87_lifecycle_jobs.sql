-- migrations/0053_v87_lifecycle_jobs.sql
-- v8.7: lifecycle jobs ledger (cron/scheduler visibility)

BEGIN;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.discovery_lifecycle_jobs (
  id bigserial PRIMARY KEY,

  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  job_type varchar(32) NOT NULL, -- mark_stale|schedule_retry|schedule_apply_retry|archive_expired|requeue_review
  job_key varchar(64) NOT NULL,  -- sha256 deterministic
  status varchar(16) NOT NULL DEFAULT 'running', -- running|done|failed

  stats jsonb NOT NULL DEFAULT '{}'::jsonb, -- small numbers only (scanned/changed/etc)
  trace_id varchar(128) NOT NULL,
  run_id uuid NOT NULL REFERENCES public.runs(run_id) ON DELETE RESTRICT,

  started_at timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz NULL,

  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT lifecycle_jobs_key_len CHECK (length(job_key)=64),
  CONSTRAINT lifecycle_jobs_status_ck CHECK (lower(status) IN ('running','done','failed')),
  CONSTRAINT lifecycle_jobs_trace_nonempty CHECK (btrim(trace_id) <> '')
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_discovery_lifecycle_jobs_v87') THEN
    ALTER TABLE public.discovery_lifecycle_jobs
      ADD CONSTRAINT ux_discovery_lifecycle_jobs_v87 UNIQUE (project_id, job_key);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_lifecycle_jobs_v87_project_time
  ON public.discovery_lifecycle_jobs(project_id, started_at DESC);

COMMIT;