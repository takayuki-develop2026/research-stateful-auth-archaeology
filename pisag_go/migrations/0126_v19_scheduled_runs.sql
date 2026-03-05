BEGIN;

CREATE TABLE IF NOT EXISTS scheduled_runs (
  id BIGSERIAL PRIMARY KEY,
  project_id VARCHAR NOT NULL,
  schedule_id BIGINT NOT NULL REFERENCES run_schedules(id) ON DELETE CASCADE,

  scheduled_for_utc TIMESTAMPTZ NOT NULL,
  trace_id TEXT NOT NULL,

  -- queued|dispatched|skipped_budget|skipped_policy|error
  status TEXT NOT NULL CHECK (status IN ('queued','dispatched','skipped_budget','skipped_policy','error')),

  reason_code TEXT NULL,
  reason_evidence_asset_id BIGINT NULL,
  error_detail_evidence_asset_id BIGINT NULL,

  run_id TEXT NULL, -- run_id is text in your Go core (v41). keep text.
  enqueued_at_utc TIMESTAMPTZ NOT NULL DEFAULT now(),
  dispatched_at_utc TIMESTAMPTZ NULL,

  -- DB冪等（A3/二重enqueue対策）
  enqueue_key TEXT NOT NULL,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;