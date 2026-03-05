BEGIN;

-- runs table: add scheduler-related columns if missing
ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS schedule_id BIGINT NULL,
  ADD COLUMN IF NOT EXISTS scheduled_run_id BIGINT NULL,
  ADD COLUMN IF NOT EXISTS replay_of_run_id TEXT NULL,

  ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS retry_of_run_id TEXT NULL,
  ADD COLUMN IF NOT EXISTS next_retry_at_utc TIMESTAMPTZ NULL,

  ADD COLUMN IF NOT EXISTS failure_state TEXT NULL,
  ADD COLUMN IF NOT EXISTS failure_reason_code TEXT NULL,
  ADD COLUMN IF NOT EXISTS failure_detail_evidence_asset_id BIGINT NULL,

  ADD COLUMN IF NOT EXISTS policy_version_id TEXT NULL,
  ADD COLUMN IF NOT EXISTS pipeline_version TEXT NULL,
  ADD COLUMN IF NOT EXISTS mode TEXT NULL,

  ADD COLUMN IF NOT EXISTS input_hash TEXT NULL;

-- tighten constraints carefully (only if safe for your data)
-- enforce trace_id NOT NULL is already v3; skip here.

COMMIT;