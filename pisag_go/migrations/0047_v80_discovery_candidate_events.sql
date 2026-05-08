-- migrations/0047_v80_discovery_candidate_events.sql
-- v8.0: lightweight audit for candidates (no big payload; evidence_ref for details)

BEGIN;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.discovery_candidate_events (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  candidate_id bigint NOT NULL REFERENCES public.discovery_candidates(id) ON DELETE CASCADE,

  event_type varchar(48) NOT NULL, -- proposed|review_requested|approved|edited|rejected|applied|retry_scheduled|conflict|contract_violation
  actor_type varchar(16) NOT NULL, -- system|user|service
  actor_id varchar(128) NULL,

  note_evidence_ref uuid NULL, -- details in evidence_assets via evidence_ref
  trace_id varchar(128) NOT NULL,
  run_id uuid NOT NULL REFERENCES public.runs(run_id) ON DELETE RESTRICT,

  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT discovery_candidate_events_actor_ck CHECK (lower(actor_type) IN ('system','user','service')),
  CONSTRAINT discovery_candidate_events_type_nonempty CHECK (btrim(event_type) <> '')
);

CREATE INDEX IF NOT EXISTS idx_candidate_events_v80_project_candidate_time
  ON public.discovery_candidate_events(project_id, candidate_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_candidate_events_v80_trace
  ON public.discovery_candidate_events(trace_id);

COMMIT;