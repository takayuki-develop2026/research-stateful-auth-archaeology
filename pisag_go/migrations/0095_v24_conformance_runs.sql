-- 0095_v24_conformance_runs.sql
-- v24 Phase A: Conformance test results (release gate)
-- Uses run_id/trace_id as first-class fields (v3 alignment).
BEGIN;

CREATE TABLE IF NOT EXISTS public.conformance_runs (
  id BIGSERIAL PRIMARY KEY,

  implementation TEXT NOT NULL, -- e.g. 'go-decisioncore', 'laravel', 'springboot'

  contract_release_id BIGINT NOT NULL,

  run_id UUID NOT NULL,
  trace_id TEXT NOT NULL,

  status TEXT NOT NULL CHECK (status IN ('passed','failed')),

  -- report artifact (JUnit/HTML/JSON bundle)
  report_artifact_ref TEXT NOT NULL,

  started_at_utc TIMESTAMPTZ NOT NULL,
  finished_at_utc TIMESTAMPTZ NOT NULL,

  created_at_utc TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT fk_conformance_runs_contract_release
    FOREIGN KEY (contract_release_id) REFERENCES public.api_contract_releases(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_conformance_runs_release_impl
  ON public.conformance_runs(contract_release_id, implementation);

CREATE INDEX IF NOT EXISTS idx_conformance_runs_status_created
  ON public.conformance_runs(status, created_at_utc);

CREATE INDEX IF NOT EXISTS idx_conformance_runs_trace
  ON public.conformance_runs(trace_id);

COMMIT;