-- 0105_v142_balance_exec_only_fns.sql
-- v14.2 exec-only:
-- - ledger_balance_snapshot_upsert_day_v14: compute + upsert daily snapshots for all active accounts in project.

BEGIN;

-- helper: jsonb array check
CREATE OR REPLACE FUNCTION public._jsonb_is_array_v14(p jsonb)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
AS $$
  SELECT COALESCE(jsonb_typeof(p) = 'array', false);
$$;

-- helper: sha256 hex
CREATE OR REPLACE FUNCTION public.sha256_hex_v14(p_text text)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$
  SELECT encode(digest(coalesce(p_text,''), 'sha256'), 'hex');
$$;

-- ============================================================
-- Core: compute + upsert daily snapshots
-- ============================================================
CREATE OR REPLACE FUNCTION public.ledger_balance_snapshot_upsert_day_v14(
  p_project_id text,
  p_as_of_date date,
  p_calc_run_id text,
  p_calc_trace_id text,
  p_policy_version_id text,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(
  affected_accounts int,
  as_of_date date
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id,''));
  v_count int := 0;
  v_day_start timestamptz;
  v_day_end timestamptz;
BEGIN
  IF v_project_id = '' THEN
    RAISE EXCEPTION 'project_id is required' USING ERRCODE='22023';
  END IF;
  IF p_as_of_date IS NULL THEN
    RAISE EXCEPTION 'as_of_date is required' USING ERRCODE='22023';
  END IF;
  IF btrim(coalesce(p_calc_run_id,'')) = '' THEN
    RAISE EXCEPTION 'calc_run_id is required' USING ERRCODE='22023';
  END IF;
  IF btrim(coalesce(p_calc_trace_id,'')) = '' THEN
    RAISE EXCEPTION 'calc_trace_id is required' USING ERRCODE='22023';
  END IF;
  IF btrim(coalesce(p_policy_version_id,'')) = '' THEN
    RAISE EXCEPTION 'policy_version_id is required' USING ERRCODE='22023';
  END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN
    RAISE EXCEPTION 'append_evidence_refs must be a json array' USING ERRCODE='22023';
  END IF;

  -- project existence
  PERFORM 1 FROM public.projects p WHERE p.id = v_project_id::varchar(26);
  IF NOT FOUND THEN
    RAISE EXCEPTION 'project not found' USING ERRCODE='23503';
  END IF;

  -- UTC day boundary contract
  v_day_start := (p_as_of_date::timestamptz);
  v_day_end   := (p_as_of_date::timestamptz) + interval '1 day';

  WITH per_account AS (
    SELECT
      a.id AS account_id,
      a.currency AS currency,
      COALESCE(SUM(CASE WHEN e.direction = 'debit'  THEN e.amount ELSE 0 END),0)::bigint AS debit_total,
      COALESCE(SUM(CASE WHEN e.direction = 'credit' THEN e.amount ELSE 0 END),0)::bigint AS credit_total
    FROM public.ledger_accounts a
    LEFT JOIN public.ledger_entries e
      ON e.account_id = a.id
     AND e.project_id = v_project_id
     AND e.created_at >= v_day_start
     AND e.created_at <  v_day_end
    WHERE a.project_id = v_project_id
      AND a.status = 'active'
    GROUP BY a.id, a.currency
  )
  INSERT INTO public.ledger_balance_snapshots(
    project_id, account_id, as_of_date, currency,
    debit_total, credit_total, balance, checksum,
    calc_run_id, calc_trace_id, policy_version_id,
    evidence_refs, created_at, updated_at
  )
  SELECT
    v_project_id,
    p.account_id,
    p_as_of_date,
    p.currency,
    p.debit_total,
    p.credit_total,
    (p.debit_total - p.credit_total)::bigint AS balance,
    public.sha256_hex_v14(
      v_project_id || '|' || p.account_id::text || '|' || p_as_of_date::text || '|' ||
      p.debit_total::text || '|' || p.credit_total::text || '|' || (p.debit_total - p.credit_total)::text
    )::char(64) AS checksum,
    p_calc_run_id,
    p_calc_trace_id,
    p_policy_version_id,
    p_append_evidence_refs,
    now(), now()
  FROM per_account p
  ON CONFLICT ON CONSTRAINT uq_lbs_project_account_day
  DO UPDATE SET
    currency = EXCLUDED.currency,
    debit_total = EXCLUDED.debit_total,
    credit_total = EXCLUDED.credit_total,
    balance = EXCLUDED.balance,
    checksum = EXCLUDED.checksum,
    calc_run_id = EXCLUDED.calc_run_id,
    calc_trace_id = EXCLUDED.calc_trace_id,
    policy_version_id = EXCLUDED.policy_version_id,
    evidence_refs = (COALESCE(public.ledger_balance_snapshots.evidence_refs,'[]'::jsonb) || EXCLUDED.evidence_refs),
    updated_at = now();

  GET DIAGNOSTICS v_count = ROW_COUNT;

  affected_accounts := v_count;
  as_of_date := p_as_of_date;
  RETURN NEXT;
END;
$$;

-- Fail-closed: revoke from PUBLIC
REVOKE ALL ON FUNCTION public._jsonb_is_array_v14(jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.sha256_hex_v14(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.ledger_balance_snapshot_upsert_day_v14(text,date,text,text,text,jsonb) FROM PUBLIC;

COMMIT;