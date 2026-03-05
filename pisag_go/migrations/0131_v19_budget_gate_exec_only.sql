BEGIN;

-- returns allowed + reason_code + remaining (daily/monthly) after considering reserved+consumed
CREATE OR REPLACE FUNCTION runsched_budget_gate_check_v19(
  p_project_id TEXT,
  p_now_utc TIMESTAMPTZ,
  p_amount BIGINT
)
RETURNS TABLE(
  allowed BOOLEAN,
  reason_code TEXT,
  remaining_daily BIGINT,
  remaining_monthly BIGINT
)
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
  v_per_run BIGINT;
  v_daily BIGINT;
  v_monthly BIGINT;

  v_day_spent BIGINT;
  v_month_spent BIGINT;

  v_day_reserved BIGINT;
  v_month_reserved BIGINT;

  v_day_total BIGINT;
  v_month_total BIGINT;
BEGIN
  SELECT per_run_limit, daily_limit, monthly_limit
    INTO v_per_run, v_daily, v_monthly
  FROM project_budgets
  WHERE project_id = p_project_id;

  IF NOT FOUND THEN
    RETURN QUERY SELECT false, 'budget_not_configured', 0, 0;
    RETURN;
  END IF;

  IF p_amount < 0 THEN
    RETURN QUERY SELECT false, 'invalid_amount', 0, 0;
    RETURN;
  END IF;

  IF v_per_run > 0 AND p_amount > v_per_run THEN
    RETURN QUERY SELECT false, 'per_run_limit_exceeded', 0, 0;
    RETURN;
  END IF;

  -- spent from ledger (credits only)
  SELECT COALESCE(SUM(amount),0) INTO v_day_spent
  FROM budget_ledger
  WHERE project_id=p_project_id
    AND unit='credits'
    AND created_at >= date_trunc('day', p_now_utc)
    AND created_at <  date_trunc('day', p_now_utc) + interval '1 day';

  SELECT COALESCE(SUM(amount),0) INTO v_month_spent
  FROM budget_ledger
  WHERE project_id=p_project_id
    AND unit='credits'
    AND created_at >= date_trunc('month', p_now_utc)
    AND created_at <  date_trunc('month', p_now_utc) + interval '1 month';

  -- reserved+consumed reservations within day/month (only credits)
  SELECT COALESCE(SUM(amount),0) INTO v_day_reserved
  FROM budget_reservations_v19
  WHERE project_id=p_project_id
    AND unit='credits'
    AND status IN ('reserved','consumed')
    AND reserved_at_utc >= date_trunc('day', p_now_utc)
    AND reserved_at_utc <  date_trunc('day', p_now_utc) + interval '1 day';

  SELECT COALESCE(SUM(amount),0) INTO v_month_reserved
  FROM budget_reservations_v19
  WHERE project_id=p_project_id
    AND unit='credits'
    AND status IN ('reserved','consumed')
    AND reserved_at_utc >= date_trunc('month', p_now_utc)
    AND reserved_at_utc <  date_trunc('month', p_now_utc) + interval '1 month';

  v_day_total := v_day_spent + v_day_reserved;
  v_month_total := v_month_spent + v_month_reserved;

  remaining_daily := GREATEST(v_daily - v_day_total, 0);
  remaining_monthly := GREATEST(v_monthly - v_month_total, 0);

  IF v_daily > 0 AND (v_day_total + p_amount) > v_daily THEN
    RETURN QUERY SELECT false, 'daily_limit_exceeded', remaining_daily, remaining_monthly;
    RETURN;
  END IF;

  IF v_monthly > 0 AND (v_month_total + p_amount) > v_monthly THEN
    RETURN QUERY SELECT false, 'monthly_limit_exceeded', remaining_daily, remaining_monthly;
    RETURN;
  END IF;

  RETURN QUERY SELECT true, '', remaining_daily, remaining_monthly;
END;
$$;

-- reserve (idempotent by project+scheduled_run_id unique)
CREATE OR REPLACE FUNCTION runsched_budget_reserve_v19(
  p_project_id TEXT,
  p_scheduled_run_id BIGINT,
  p_trace_id TEXT,
  p_amount BIGINT,
  p_reason_code TEXT,
  p_reason_evidence_asset_id BIGINT
)
RETURNS TABLE(
  reservation_id BIGINT,
  found_existing BOOLEAN
)
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
  v_id BIGINT;
BEGIN
  INSERT INTO budget_reservations_v19(
    project_id, scheduled_run_id, amount, unit, status,
    trace_id, reason_code, reason_evidence_asset_id
  )
  VALUES (
    p_project_id, p_scheduled_run_id, p_amount, 'credits', 'reserved',
    p_trace_id, p_reason_code, p_reason_evidence_asset_id
  )
  ON CONFLICT (project_id, scheduled_run_id)
  DO UPDATE SET
    updated_at = now()
  RETURNING id INTO v_id;

  -- determine found_existing: if row existed already, still returns id; we approximate using presence check
  RETURN QUERY
    SELECT v_id,
           EXISTS(SELECT 1 FROM budget_reservations_v19 WHERE project_id=p_project_id AND scheduled_run_id=p_scheduled_run_id AND id=v_id AND reserved_at_utc < now() - interval '1 second');
END;
$$;

-- consume after run created: mark reservation consumed and write budget_ledger
CREATE OR REPLACE FUNCTION runsched_budget_consume_v19(
  p_project_id TEXT,
  p_scheduled_run_id BIGINT,
  p_run_id UUID,
  p_trace_id TEXT
)
RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
  v_amount BIGINT;
BEGIN
  SELECT amount INTO v_amount
  FROM budget_reservations_v19
  WHERE project_id=p_project_id AND scheduled_run_id=p_scheduled_run_id
  FOR UPDATE;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'reservation not found project_id=% scheduled_run_id=%', p_project_id, p_scheduled_run_id;
  END IF;

  UPDATE budget_reservations_v19
  SET status='consumed',
      consumed_at_utc=now(),
      run_id=p_run_id,
      updated_at=now()
  WHERE project_id=p_project_id AND scheduled_run_id=p_scheduled_run_id;

  -- ledger: unique by (run_id, reason). Use deterministic reason.
  INSERT INTO budget_ledger(run_id, trace_id, project_id, amount, unit, reason, created_at)
  VALUES (
    p_run_id::text,
    p_trace_id,
    p_project_id,
    v_amount,
    'credits',
    'runsched.consume',
    now()
  )
  ON CONFLICT (run_id, reason) DO NOTHING;
END;
$$;

COMMIT;