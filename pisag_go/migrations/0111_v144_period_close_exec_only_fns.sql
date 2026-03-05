-- 0111_v144_period_close_exec_only_fns.sql
-- v14.4 exec-only close:
-- - ledger_period_close_v14: performs closing checks and transitions to closed if ok.
-- Checks:
-- - rebuild dry_run summary must have out_of_balance=0, currency_mismatch=0, missing_in_ledger=0
-- - balance_snapshots must exist for day close (at least 1 row for project+date)

BEGIN;

CREATE OR REPLACE FUNCTION public.ledger_period_close_v14(
  p_project_id text,
  p_period_type text,          -- day|month
  p_period_key text,           -- day: YYYY-MM-DD, month: YYYY-MM
  p_confirm boolean,           -- must be true to close
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(period_id uuid, status text, close_summary jsonb)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id,''));
  v_type text := btrim(coalesce(p_period_type,''));
  v_key text := btrim(coalesce(p_period_key,''));
  v_run_id text := btrim(coalesce(p_run_id,''));
  v_trace_id text := btrim(coalesce(p_trace_id,''));
  v_policy text := btrim(coalesce(p_policy_version_id,''));

  v_from timestamptz;
  v_to timestamptz;

  v_row public.ledger_periods%rowtype;

  v_diff jsonb;
  v_oob int := 0;
  v_cur int := 0;
  v_miss int := 0;
  v_extra int := 0;

  v_snap_count int := 0;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_type NOT IN ('day','month') THEN RAISE EXCEPTION 'period_type invalid' USING ERRCODE='22023'; END IF;
  IF v_key='' THEN RAISE EXCEPTION 'period_key required' USING ERRCODE='22023'; END IF;
  IF v_run_id='' THEN RAISE EXCEPTION 'run_id required' USING ERRCODE='22023'; END IF;
  IF v_trace_id='' THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;
  IF v_policy='' THEN RAISE EXCEPTION 'policy_version_id required' USING ERRCODE='22023'; END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN
    RAISE EXCEPTION 'append_evidence_refs must be a json array' USING ERRCODE='22023';
  END IF;

  PERFORM 1 FROM public.projects p WHERE p.id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  -- Build UTC window from period_key
  IF v_type='day' THEN
    -- expect YYYY-MM-DD
    v_from := (to_date(v_key,'YYYY-MM-DD')::timestamptz);
    v_to := (to_date(v_key,'YYYY-MM-DD')::timestamptz) + interval '1 day';
  ELSE
    -- month: YYYY-MM
    v_from := (to_date(v_key||'-01','YYYY-MM-DD')::timestamptz);
    v_to := (to_date(v_key||'-01','YYYY-MM-DD')::timestamptz) + interval '1 month';
  END IF;

  -- Upsert/lock the period row
  INSERT INTO public.ledger_periods(
    project_id, period_type, period_key,
    status, close_summary, evidence_refs,
    run_id, trace_id, policy_version_id
  )
  VALUES(
    v_project_id, v_type, v_key,
    'closing', '{}'::jsonb, p_append_evidence_refs,
    v_run_id, v_trace_id, v_policy
  )
  ON CONFLICT (project_id, period_type, period_key)
  DO UPDATE SET
    status='closing',
    run_id=EXCLUDED.run_id,
    trace_id=EXCLUDED.trace_id,
    policy_version_id=EXCLUDED.policy_version_id,
    evidence_refs=(COALESCE(public.ledger_periods.evidence_refs,'[]'::jsonb) || EXCLUDED.evidence_refs),
    updated_at=now()
  RETURNING * INTO v_row;

  -- If confirm is not true, do not close; record summary and return
  IF p_confirm IS DISTINCT FROM true THEN
    v_row.close_summary := jsonb_build_object(
      'window', jsonb_build_object('from', v_from, 'to', v_to),
      'confirm_required', true
    );

    UPDATE public.ledger_periods
       SET status='open',
           close_summary=v_row.close_summary,
           updated_at=now()
     WHERE id=v_row.id;

    period_id := v_row.id;
    status := 'open';
    close_summary := v_row.close_summary;
    RETURN NEXT;
    RETURN;
  END IF;

  -- Run rebuild dry_run compute (v14.3.1)
  v_diff := public.ledger_rebuild_run_dry_run_compute_v14(v_project_id, v_from, v_to);

  v_oob  := COALESCE((v_diff->>'out_of_balance_count')::int, 0);
  v_cur  := COALESCE((v_diff->>'currency_mismatch_count')::int, 0);
  v_miss := COALESCE((v_diff->>'missing_in_ledger_count')::int, 0);
  v_extra:= COALESCE((v_diff->>'extra_in_ledger_count')::int, 0);

  -- Balance snapshot existence check (day close only)
  IF v_type='day' THEN
    SELECT count(*) INTO v_snap_count
    FROM public.ledger_balance_snapshots s
    WHERE s.project_id=v_project_id
      AND s.as_of_date = to_date(v_key,'YYYY-MM-DD');
  ELSE
    v_snap_count := 1; -- month close: not enforced in v14.4 P0
  END IF;

  v_row.close_summary := jsonb_build_object(
    'window', (v_diff->'window'),
    'out_of_balance_count', v_oob,
    'currency_mismatch_count', v_cur,
    'missing_in_ledger_count', v_miss,
    'extra_in_ledger_count', v_extra,
    'balance_snapshots_count', v_snap_count,
    'diff', v_diff
  );

  IF v_oob=0 AND v_cur=0 AND v_miss=0 AND v_extra=0 AND v_snap_count>0 THEN
    UPDATE public.ledger_periods
       SET status='closed',
           close_summary=v_row.close_summary,
           closed_at=now(),
           closed_by='system',
           updated_at=now()
     WHERE id=v_row.id;

    period_id := v_row.id;
    status := 'closed';
    close_summary := v_row.close_summary;
    RETURN NEXT;
    RETURN;
  END IF;

  -- Fail recorded (no throw)
  UPDATE public.ledger_periods
     SET status='failed_recorded',
         close_summary=v_row.close_summary,
         updated_at=now()
   WHERE id=v_row.id;

  period_id := v_row.id;
  status := 'failed_recorded';
  close_summary := v_row.close_summary;
  RETURN NEXT;
END;
$$;

REVOKE ALL ON FUNCTION public.ledger_period_close_v14(text,text,text,boolean,text,text,text,jsonb) FROM PUBLIC;

COMMIT;