-- migrations/0002_v42_claim_run_inputs.sql
-- 목적: run_inputs を “DB single-owner claim” 可能に拡張（idは既に0001で定義済み）
-- 注意: 0001_runs_v41.sql で run_inputs.id(bigserial PK) が存在する前提。

BEGIN;

-- claim fields
ALTER TABLE public.run_inputs
  ADD COLUMN IF NOT EXISTS claim_status       text        NOT NULL DEFAULT 'pending',
  ADD COLUMN IF NOT EXISTS claimed_at         timestamptz NULL,
  ADD COLUMN IF NOT EXISTS claimed_by         text        NULL,
  ADD COLUMN IF NOT EXISTS attempt_count      int         NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS next_attempt_at    timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS last_error_code    text        NULL,
  ADD COLUMN IF NOT EXISTS last_error_message text        NULL;

-- queue ordering: 0001 で created_at は既にあるが、万一古い環境で無ければ追加
ALTER TABLE public.run_inputs
  ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();

-- queue index: pending + due + stable ordering
CREATE INDEX IF NOT EXISTS run_inputs_claim_queue_idx
  ON public.run_inputs (claim_status, next_attempt_at, created_at, id);

-- who owns the claim (debug/ops)
CREATE INDEX IF NOT EXISTS run_inputs_claimed_by_idx
  ON public.run_inputs (claimed_by)
  WHERE claim_status = 'claimed';

-- helpful for joins
CREATE INDEX IF NOT EXISTS run_inputs_run_id_idx
  ON public.run_inputs (run_id);

COMMIT;