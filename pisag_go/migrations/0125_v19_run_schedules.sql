BEGIN;

-- Needed for sha256 digest / gen_random_uuid
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS run_schedules (
  id BIGSERIAL PRIMARY KEY,
  project_id VARCHAR NOT NULL,

  name TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,

  -- 'cron'|'interval'|'manual_trigger'
  schedule_kind TEXT NOT NULL CHECK (schedule_kind IN ('cron','interval','manual_trigger')),
  cron_expr TEXT NULL,
  interval_seconds INTEGER NULL CHECK (interval_seconds IS NULL OR interval_seconds > 0),
  timezone TEXT NOT NULL DEFAULT 'UTC',

  task_type TEXT NOT NULL,
  pipeline_version TEXT NOT NULL,
  policy_version_id TEXT NOT NULL,
  mode TEXT NULL,

  priority INTEGER NOT NULL DEFAULT 50 CHECK (priority >= 0 AND priority <= 100),

  -- E条文対象外の「原子値」：評価に必須なのでSoT保持を許容（v19契約の例外条文）
  next_run_at_utc TIMESTAMPTZ NOT NULL,
  last_run_at_utc TIMESTAMPTZ NULL,

  -- E条文：本文は evidence_asset_id 参照のみ（JSON列禁止）
  input_template_evidence_asset_id BIGINT NULL,
  budget_policy_evidence_asset_id BIGINT NOT NULL,
  retry_policy_evidence_asset_id BIGINT NOT NULL,

  -- concurrency policy
  concurrency_policy TEXT NOT NULL DEFAULT 'allow'
    CHECK (concurrency_policy IN ('allow','forbid','replace','singleton')),
  max_concurrent_runs INTEGER NULL CHECK (max_concurrent_runs IS NULL OR max_concurrent_runs > 0),

  created_by_type TEXT NOT NULL DEFAULT 'system'
    CHECK (created_by_type IN ('system','user')),
  created_by_id VARCHAR NULL,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- simple updated_at trigger (optional; safe)
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'set_updated_at_v19') THEN
    CREATE FUNCTION set_updated_at_v19() RETURNS trigger AS $fn$
    BEGIN
      NEW.updated_at = now();
      RETURN NEW;
    END;
    $fn$ LANGUAGE plpgsql;
  END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_run_schedules_updated_at_v19') THEN
    CREATE TRIGGER trg_run_schedules_updated_at_v19
    BEFORE UPDATE ON run_schedules
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at_v19();
  END IF;
END$$;

COMMIT;