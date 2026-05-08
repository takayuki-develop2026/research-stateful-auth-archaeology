-- 0104_v142_ledger_balance_snapshots.sql
-- v14.2: daily balance snapshots (contract for v15+)
-- SoT remains ledger_entries; snapshots are operational contract + close input.

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.ledger_balance_snapshots (
  id bigserial PRIMARY KEY,
  project_id text NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  account_id uuid NOT NULL REFERENCES public.ledger_accounts(id) ON DELETE RESTRICT,

  as_of_date date NOT NULL,                -- day boundary in UTC (contract)
  currency text NOT NULL,

  debit_total bigint NOT NULL,
  credit_total bigint NOT NULL,
  balance bigint NOT NULL,                 -- debit - credit (raw)
  checksum char(64) NOT NULL,              -- sha256 hex

  calc_run_id text NOT NULL,
  calc_trace_id text NOT NULL,
  policy_version_id text NOT NULL,

  evidence_refs jsonb NOT NULL DEFAULT '[]'::jsonb, -- evidence_ref UUID strings (json array)
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT lbs_currency_nonempty CHECK (btrim(currency) <> ''),
  CONSTRAINT lbs_checksum_len CHECK (length(checksum) = 64),
  CONSTRAINT lbs_evidence_refs_is_array CHECK (jsonb_typeof(evidence_refs) = 'array')
);

-- Unique index (first)
CREATE UNIQUE INDEX IF NOT EXISTS ux_lbs_project_account_day
  ON public.ledger_balance_snapshots(project_id, account_id, as_of_date);

-- ✅ IMPORTANT: add a named UNIQUE CONSTRAINT using the index (for ON CONFLICT ON CONSTRAINT)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
    WHERE n.nspname='public'
      AND t.relname='ledger_balance_snapshots'
      AND c.conname='uq_lbs_project_account_day'
  ) THEN
    ALTER TABLE public.ledger_balance_snapshots
      ADD CONSTRAINT uq_lbs_project_account_day
      UNIQUE USING INDEX ux_lbs_project_account_day;
  END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_lbs_project_day
  ON public.ledger_balance_snapshots(project_id, as_of_date DESC);

CREATE INDEX IF NOT EXISTS idx_lbs_project_account
  ON public.ledger_balance_snapshots(project_id, account_id, as_of_date DESC);

COMMIT;