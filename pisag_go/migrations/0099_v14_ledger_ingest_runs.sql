-- 0099_v14_ledger_ingest_runs.sql
-- v14.1: run-based UTL -> Ledger ingest execution ledger (SoT for orchestration runs)
-- Policy:
-- - idempotent by (project_id, idempotency_key)
-- - status is recorded, never throw for internal failures (use failed_recorded)
-- - evidence_refs is JSON array (IDs of evidence_assets)

BEGIN;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'ledger_ingest_mode_v14') THEN
    CREATE TYPE ledger_ingest_mode_v14 AS ENUM ('single_event','range');
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'ledger_ingest_status_v14') THEN
    CREATE TYPE ledger_ingest_status_v14 AS ENUM ('accepted','running','succeeded','failed_recorded');
  END IF;
END$$;

CREATE TABLE IF NOT EXISTS ledger_ingest_runs (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id        text NOT NULL,

  mode             ledger_ingest_mode_v14 NOT NULL,
  source_event_key text NULL,          -- for single_event
  from_ts          timestamptz NULL,   -- for range
  to_ts            timestamptz NULL,   -- for range
  filter           jsonb NOT NULL DEFAULT '{}'::jsonb,

  idempotency_key  text NOT NULL,      -- required for orchestration endpoints
  status           ledger_ingest_status_v14 NOT NULL DEFAULT 'accepted',

  run_id           text NOT NULL,
  trace_id         text NOT NULL,
  policy_version_id text NOT NULL,

  stats            jsonb NOT NULL DEFAULT '{}'::jsonb, -- {event_count, posted_count, already_exists_count, failed_count, ...}
  evidence_refs    jsonb NOT NULL DEFAULT '[]'::jsonb, -- array of evidence_asset_id (mapping report / reject list)

  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT ledger_ingest_runs_idempotency_key_len CHECK (length(trim(idempotency_key)) > 0),
  CONSTRAINT ledger_ingest_runs_trace_id_len CHECK (length(trim(trace_id)) > 0),
  CONSTRAINT ledger_ingest_runs_run_id_len CHECK (length(trim(run_id)) > 0),
  CONSTRAINT ledger_ingest_runs_policy_version_id_len CHECK (length(trim(policy_version_id)) > 0),
  CONSTRAINT ledger_ingest_runs_filter_is_object CHECK (jsonb_typeof(filter) = 'object'),
  CONSTRAINT ledger_ingest_runs_stats_is_object CHECK (jsonb_typeof(stats) = 'object'),
  CONSTRAINT ledger_ingest_runs_evidence_refs_is_array CHECK (jsonb_typeof(evidence_refs) = 'array'),
  CONSTRAINT ledger_ingest_runs_mode_scope CHECK (
    (mode = 'single_event' AND source_event_key IS NOT NULL AND from_ts IS NULL AND to_ts IS NULL)
    OR
    (mode = 'range' AND source_event_key IS NULL AND from_ts IS NOT NULL AND to_ts IS NOT NULL AND from_ts < to_ts)
  )
);

-- idempotency
CREATE UNIQUE INDEX IF NOT EXISTS ux_ledger_ingest_runs_project_idempotency_key
  ON ledger_ingest_runs(project_id, idempotency_key);

CREATE INDEX IF NOT EXISTS ix_ledger_ingest_runs_project_status_created
  ON ledger_ingest_runs(project_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS ix_ledger_ingest_runs_project_source_event_key
  ON ledger_ingest_runs(project_id, source_event_key);

CREATE INDEX IF NOT EXISTS ix_ledger_ingest_runs_project_range
  ON ledger_ingest_runs(project_id, from_ts, to_ts);

COMMIT;