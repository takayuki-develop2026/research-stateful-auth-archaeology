-- migrations/0001_runs_v41.sql
-- v4.1: Fetch Run minimal schema (runs + run_events + run_inputs)

CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'run_status') THEN
    CREATE TYPE run_status AS ENUM ('running', 'done', 'failed');
  END IF;
END$$;

CREATE TABLE IF NOT EXISTS runs (
  run_id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id       text NOT NULL,
  trace_id         uuid NOT NULL DEFAULT gen_random_uuid(),
  pipeline_version text NOT NULL DEFAULT 'v4.1',
  status           run_status NOT NULL DEFAULT 'running',

  started_at       timestamptz NOT NULL DEFAULT now(),
  finished_at      timestamptz NULL,

  error_code       text NULL,
  error_message    text NULL,

  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_runs_project_created
  ON runs(project_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_runs_trace
  ON runs(trace_id);

CREATE TABLE IF NOT EXISTS run_inputs (
  id              bigserial PRIMARY KEY,
  run_id          uuid NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,

  source_id       text NULL,
  target_url      text NOT NULL,
  method          text NOT NULL DEFAULT 'GET',
  headers_json    jsonb NOT NULL DEFAULT '{}'::jsonb,

  allowlist_key   text NULL,

  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_run_inputs_run
  ON run_inputs(run_id);

CREATE TABLE IF NOT EXISTS run_events (
  id              bigserial PRIMARY KEY,
  run_id          uuid NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,

  trace_id        uuid NOT NULL,
  event_name      text NOT NULL,   -- e.g. fetch_started/fetch_done/fetch_failed
  step            text NOT NULL,   -- e.g. fetch
  status          text NOT NULL,   -- e.g. ok/failed
  message         text NULL,
  data_json       jsonb NOT NULL DEFAULT '{}'::jsonb,

  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_run_events_run_created
  ON run_events(run_id, created_at ASC);

CREATE INDEX IF NOT EXISTS idx_run_events_trace_created
  ON run_events(trace_id, created_at ASC);

-- updated_at auto update trigger
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger WHERE tgname = 'trg_runs_updated_at'
  ) THEN
    CREATE TRIGGER trg_runs_updated_at
    BEFORE UPDATE ON runs
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
  END IF;
END$$;