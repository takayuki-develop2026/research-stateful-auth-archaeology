BEGIN;

CREATE TABLE IF NOT EXISTS public.compliance_events_v21 (
  id BIGSERIAL PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  trace_id text NOT NULL,

  event_type text NOT NULL,
  event_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  primary_artifact_asset_id bigint NULL REFERENCES public.artifact_assets(id) ON DELETE SET NULL,

  created_at_utc timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT ce_v21_project_nonempty CHECK (btrim(project_id::text) <> ''),
  CONSTRAINT ce_v21_trace_nonempty CHECK (btrim(trace_id) <> ''),
  CONSTRAINT ce_v21_type_nonempty CHECK (btrim(event_type) <> '')
);

CREATE INDEX IF NOT EXISTS idx_compliance_events_v21_project_time
  ON public.compliance_events_v21(project_id, created_at_utc DESC);

CREATE INDEX IF NOT EXISTS idx_compliance_events_v21_trace
  ON public.compliance_events_v21(trace_id);

COMMIT;