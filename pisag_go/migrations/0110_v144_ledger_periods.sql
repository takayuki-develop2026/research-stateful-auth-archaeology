-- 0110_v144_ledger_periods.sql
-- v14.4: minimal period close table (day/month)
-- close_summary stores the last close check result (json object).
-- evidence_refs stores evidence_ref UUID strings (json array).

BEGIN;

CREATE TABLE IF NOT EXISTS public.ledger_periods (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id text NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  period_type text NOT NULL,    -- day|month
  period_key  text NOT NULL,    -- day: YYYY-MM-DD, month: YYYY-MM

  status text NOT NULL,         -- open|closing|closed|failed_recorded
  close_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
  evidence_refs jsonb NOT NULL DEFAULT '[]'::jsonb,

  closed_at timestamptz NULL,
  closed_by text NULL,

  run_id text NOT NULL,
  trace_id text NOT NULL,
  policy_version_id text NOT NULL,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT lp_period_type_ck CHECK (period_type IN ('day','month')),
  CONSTRAINT lp_status_ck CHECK (status IN ('open','closing','closed','failed_recorded')),
  CONSTRAINT lp_close_summary_is_object CHECK (jsonb_typeof(close_summary)='object'),
  CONSTRAINT lp_evidence_refs_is_array CHECK (jsonb_typeof(evidence_refs)='array')
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_lp_project_type_key
  ON public.ledger_periods(project_id, period_type, period_key);

CREATE INDEX IF NOT EXISTS idx_lp_project_status_time
  ON public.ledger_periods(project_id, status, created_at DESC);

COMMIT;