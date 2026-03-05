-- 0107_v143_ledger_rebuild_runs.sql
-- v14.3: rebuild runs (dry_run/apply) execution ledger

BEGIN;

CREATE TABLE IF NOT EXISTS public.ledger_rebuild_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id text NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  mode text NOT NULL, -- dry_run | apply
  from_ts timestamptz NULL,
  to_ts timestamptz NULL,

  status text NOT NULL, -- accepted | running | succeeded | failed_recorded
  requested_by text NULL,
  approved_by text NULL,
  confirm boolean NOT NULL DEFAULT false,

  run_id text NOT NULL,
  trace_id text NOT NULL,
  policy_version_id text NOT NULL,

  diff_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
  evidence_refs jsonb NOT NULL DEFAULT '[]'::jsonb, -- evidence_ref UUID strings

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT lrr_mode_ck CHECK (mode IN ('dry_run','apply')),
  CONSTRAINT lrr_status_ck CHECK (status IN ('accepted','running','succeeded','failed_recorded')),
  CONSTRAINT lrr_evidence_refs_is_array CHECK (jsonb_typeof(evidence_refs)='array'),
  CONSTRAINT lrr_diff_summary_is_object CHECK (jsonb_typeof(diff_summary)='object')
);

CREATE INDEX IF NOT EXISTS idx_lrr_project_created
  ON public.ledger_rebuild_runs(project_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_lrr_project_status
  ON public.ledger_rebuild_runs(project_id, status, created_at DESC);

COMMIT;