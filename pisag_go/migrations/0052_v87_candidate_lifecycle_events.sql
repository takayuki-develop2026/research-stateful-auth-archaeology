-- migrations/0052_v87_candidate_lifecycle_events.sql
-- v8.7: lifecycle events ledger (ops/audit; no big payload; evidence_ref for details)

BEGIN;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.discovery_candidate_lifecycle_events (
  id bigserial PRIMARY KEY,

  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,
  candidate_id bigint NOT NULL REFERENCES public.discovery_candidates(id) ON DELETE CASCADE,

  event_type varchar(48) NOT NULL, -- seen|stale_marked|stale_cleared|review_requeued|retry_scheduled|retry_exhausted|archived|unarchived|apply_retry_scheduled|apply_exhausted
  actor_type varchar(16) NOT NULL, -- system|user|service
  actor_id varchar(128) NULL,

  message varchar(256) NULL,
  detail_evidence_ref uuid NULL, -- full detail in evidence_assets

  trace_id varchar(128) NOT NULL,
  run_id uuid NOT NULL REFERENCES public.runs(run_id) ON DELETE RESTRICT,

  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT lifecycle_actor_ck CHECK (lower(actor_type) IN ('system','user','service')),
  CONSTRAINT lifecycle_event_nonempty CHECK (btrim(event_type) <> ''),
  CONSTRAINT lifecycle_trace_nonempty CHECK (btrim(trace_id) <> '')
);

CREATE INDEX IF NOT EXISTS idx_lifecycle_events_v87_project_candidate_time
  ON public.discovery_candidate_lifecycle_events(project_id, candidate_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_lifecycle_events_v87_project_type_time
  ON public.discovery_candidate_lifecycle_events(project_id, event_type, created_at DESC);

COMMIT;