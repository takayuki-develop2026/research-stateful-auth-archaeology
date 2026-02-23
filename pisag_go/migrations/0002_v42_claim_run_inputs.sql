ALTER TABLE run_inputs
  ADD COLUMN IF NOT EXISTS id bigserial PRIMARY KEY;

ALTER TABLE run_inputs
  ADD COLUMN IF NOT EXISTS claim_status text NOT NULL DEFAULT 'pending',
  ADD COLUMN IF NOT EXISTS claimed_at timestamptz,
  ADD COLUMN IF NOT EXISTS claimed_by text,
  ADD COLUMN IF NOT EXISTS attempt_count int NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS next_attempt_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS last_error_code text,
  ADD COLUMN IF NOT EXISTS last_error_message text;

-- optional: queue ordering (if you don't already have created_at)
ALTER TABLE run_inputs
  ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();

-- queue index: find pending and due (id included for stable ordering)
CREATE INDEX IF NOT EXISTS run_inputs_claim_queue_idx
  ON run_inputs (claim_status, next_attempt_at, created_at, id);

-- who owns the claim (debug/ops)
CREATE INDEX IF NOT EXISTS run_inputs_claimed_by_idx
  ON run_inputs (claimed_by)
  WHERE claim_status = 'claimed';

-- helpful for joins
CREATE INDEX IF NOT EXISTS run_inputs_run_id_idx
  ON run_inputs (run_id);