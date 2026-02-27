-- migrations/0045_v80_discovery_sources.sql
-- v8.0: discovery_sources (ingest truth / non-growing by UNIQUE)
-- E-clause aligned: NO body/json payloads. references only.
-- Integrates: v18 projects(id varchar(26)), runs(run_id uuid)

BEGIN;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.discovery_sources (
  id bigserial PRIMARY KEY,

  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  -- input
  source_type varchar(32) NOT NULL, -- pisag_html|pisag_pdf|webhook|manual|import|other
  source_ref_raw text NOT NULL,
  source_ref text NOT NULL,

  -- convergence key (caller must compute deterministic hash)
  source_hash varchar(64) NOT NULL,

  -- run/trace (v20 style)
  run_id uuid NOT NULL REFERENCES public.runs(run_id) ON DELETE RESTRICT,
  trace_id varchar(128) NOT NULL,

  pipeline_version varchar(32) NOT NULL,
  policy_version varchar(32) NOT NULL,

  -- progress vs failure split (no downgrade)
  status varchar(16) NOT NULL DEFAULT 'detected',       -- detected|acquired|extracted
  failure_state varchar(16) NOT NULL DEFAULT 'none',    -- none|needs_retry|failed
  failure_code varchar(64) NULL,
  failure_message varchar(256) NULL, -- short message only (full detail -> evidence_assets)

  -- ops metadata only (small, optional)
  acquire_metadata jsonb NOT NULL DEFAULT '{}'::jsonb,

  -- lifecycle-ish
  first_seen_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at  timestamptz NOT NULL DEFAULT now(),
  seen_count    bigint NOT NULL DEFAULT 0,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT discovery_sources_source_hash_len CHECK (length(source_hash) = 64),
  CONSTRAINT discovery_sources_source_ref_nonempty CHECK (btrim(source_ref) <> ''),
  CONSTRAINT discovery_sources_source_ref_raw_nonempty CHECK (btrim(source_ref_raw) <> ''),
  CONSTRAINT discovery_sources_status_ck CHECK (lower(status) IN ('detected','acquired','extracted')),
  CONSTRAINT discovery_sources_failure_state_ck CHECK (lower(failure_state) IN ('none','needs_retry','failed')),
  CONSTRAINT discovery_sources_trace_nonempty CHECK (btrim(trace_id) <> ''),
  CONSTRAINT discovery_sources_pipeline_nonempty CHECK (btrim(pipeline_version) <> ''),
  CONSTRAINT discovery_sources_policy_nonempty CHECK (btrim(policy_version) <> '')
);

-- non-growing
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_discovery_sources_v80') THEN
    ALTER TABLE public.discovery_sources
      ADD CONSTRAINT ux_discovery_sources_v80 UNIQUE (project_id, source_type, source_hash);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_discovery_sources_v80_project_status_time
  ON public.discovery_sources(project_id, status, last_seen_at DESC);

CREATE INDEX IF NOT EXISTS idx_discovery_sources_v80_project_failure_time
  ON public.discovery_sources(project_id, failure_state, last_seen_at DESC);

CREATE INDEX IF NOT EXISTS idx_discovery_sources_v80_project_hash
  ON public.discovery_sources(project_id, source_hash);

CREATE INDEX IF NOT EXISTS idx_discovery_sources_v80_run
  ON public.discovery_sources(project_id, run_id);

-- updated_at trigger (if exists)
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname='set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_discovery_sources_updated_at') THEN
      CREATE TRIGGER trg_discovery_sources_updated_at
      BEFORE UPDATE ON public.discovery_sources
      FOR EACH ROW
      EXECUTE FUNCTION set_updated_at();
    END IF;
  END IF;
END$$;

COMMIT;