BEGIN;

CREATE TABLE IF NOT EXISTS budget_reservations_v19 (
  id BIGSERIAL PRIMARY KEY,
  project_id TEXT NOT NULL,
  scheduled_run_id BIGINT NOT NULL,
  amount BIGINT NOT NULL,
  unit TEXT NOT NULL DEFAULT 'credits',
  status TEXT NOT NULL CHECK (status IN ('reserved','consumed','released')),
  reserved_at_utc TIMESTAMPTZ NOT NULL DEFAULT now(),
  consumed_at_utc TIMESTAMPTZ NULL,
  released_at_utc TIMESTAMPTZ NULL,
  run_id UUID NULL,
  trace_id TEXT NOT NULL,
  reason_code TEXT NOT NULL,
  reason_evidence_asset_id BIGINT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT budget_reservations_v19_amount_nonneg CHECK (amount >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_budget_reservations_v19_project_scheduled
ON budget_reservations_v19(project_id, scheduled_run_id);

CREATE INDEX IF NOT EXISTS idx_budget_reservations_v19_project_status_time
ON budget_reservations_v19(project_id, status, reserved_at_utc);

-- updated_at trigger exists already in your DB (set_updated_at). reuse it if present.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname='set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_budget_reservations_v19_updated_at') THEN
      CREATE TRIGGER trg_budget_reservations_v19_updated_at
      BEFORE UPDATE ON budget_reservations_v19
      FOR EACH ROW
      EXECUTE FUNCTION set_updated_at();
    END IF;
  END IF;
END$$;

COMMIT;