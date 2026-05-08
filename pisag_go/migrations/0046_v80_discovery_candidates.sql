-- migrations/0046_v80_discovery_candidates.sql
-- v8.0: discovery_candidates (proposal ledger; references by evidence_ref uuid only)
-- Integrates: v18 projects, v8 sources, v18 links (role allowlist)

BEGIN;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.discovery_candidates (
  id bigserial PRIMARY KEY,

  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  source_id bigint NOT NULL REFERENCES public.discovery_sources(id) ON DELETE RESTRICT,

  candidate_type varchar(32) NOT NULL, -- provider|provider_route|fee_model|catalog_source
  candidate_key  varchar(64) NOT NULL, -- sha256hex stable key (v8.2 style)

  status varchar(24) NOT NULL DEFAULT 'proposed', -- proposed|review_required|approved|rejected|applied|needs_retry|conflict
  risk_level varchar(16) NOT NULL DEFAULT 'normal', -- low|normal|high
  confidence numeric NULL, -- 0..1 optional

  -- Evidence refs (uuid) - align with v18 links functions
  payload_evidence_ref    uuid NULL,
  normalized_evidence_ref uuid NULL,
  diff_evidence_ref       uuid NULL,

  -- Optional: link to v8.4 dedupe
  dedupe_key varchar(64) NULL,
  dedupe_group_id bigint NULL,

  -- ops/lifecycle (v8.7 extends later; keep minimal here)
  first_seen_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at  timestamptz NOT NULL DEFAULT now(),
  seen_count    bigint NOT NULL DEFAULT 0,

  review_requested_at timestamptz NULL,
  decided_at          timestamptz NULL,

  run_id uuid NOT NULL REFERENCES public.runs(run_id) ON DELETE RESTRICT,
  trace_id varchar(128) NOT NULL,
  pipeline_version varchar(32) NOT NULL,
  policy_version varchar(32) NOT NULL,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT discovery_candidates_key_len CHECK (length(candidate_key)=64),
  CONSTRAINT discovery_candidates_status_ck CHECK (lower(status) IN (
    'proposed','review_required','approved','rejected','applied','needs_retry','conflict'
  )),
  CONSTRAINT discovery_candidates_risk_ck CHECK (lower(risk_level) IN ('low','normal','high')),
  CONSTRAINT discovery_candidates_trace_nonempty CHECK (btrim(trace_id) <> ''),
  CONSTRAINT discovery_candidates_pipeline_nonempty CHECK (btrim(pipeline_version) <> ''),
  CONSTRAINT discovery_candidates_policy_nonempty CHECK (btrim(policy_version) <> '')
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_discovery_candidates_v80') THEN
    ALTER TABLE public.discovery_candidates
      ADD CONSTRAINT ux_discovery_candidates_v80 UNIQUE (project_id, candidate_type, candidate_key);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_discovery_candidates_v80_project_status_time
  ON public.discovery_candidates(project_id, status, last_seen_at DESC);

CREATE INDEX IF NOT EXISTS idx_discovery_candidates_v80_project_source
  ON public.discovery_candidates(project_id, source_id);

CREATE INDEX IF NOT EXISTS idx_discovery_candidates_v80_project_dedupe
  ON public.discovery_candidates(project_id, dedupe_key);

CREATE INDEX IF NOT EXISTS idx_discovery_candidates_v80_run
  ON public.discovery_candidates(project_id, run_id);

-- updated_at trigger
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname='set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_discovery_candidates_updated_at') THEN
      CREATE TRIGGER trg_discovery_candidates_updated_at
      BEFORE UPDATE ON public.discovery_candidates
      FOR EACH ROW
      EXECUTE FUNCTION set_updated_at();
    END IF;
  END IF;
END$$;

COMMIT;