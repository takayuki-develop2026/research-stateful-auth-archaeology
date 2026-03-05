-- 0114_v143_rebuild_apply_exec_only_fns.sql
-- v14.3 apply (P0): default deny unless diff is clean (noop apply)
-- establishes governance contract; real adjustments come in v14.3.2+

BEGIN;

CREATE OR REPLACE FUNCTION public.ledger_rebuild_apply_v14(
  p_project_id text,
  p_from_ts timestamptz,
  p_to_ts timestamptz,
  p_confirm boolean,
  p_requested_by text,
  p_approved_by text,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(rebuild_run_id uuid, status text, diff_summary jsonb)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id,''));
  v_run_id text := btrim(coalesce(p_run_id,''));
  v_trace_id text := btrim(coalesce(p_trace_id,''));
  v_policy text := btrim(coalesce(p_policy_version_id,''));
  v_req text := NULLIF(btrim(coalesce(p_requested_by,'')), '');
  v_appr text := NULLIF(btrim(coalesce(p_approved_by,'')), '');

  v_diff jsonb;
  v_oob int := 0;
  v_cur int := 0;
  v_miss int := 0;
  v_extra int := 0;

  v_id uuid;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF p_from_ts IS NULL OR p_to_ts IS NULL OR NOT (p_from_ts < p_to_ts) THEN
    RAISE EXCEPTION 'valid from/to required' USING ERRCODE='22023'; END IF;
  IF v_run_id='' THEN RAISE EXCEPTION 'run_id required' USING ERRCODE='22023'; END IF;
  IF v_trace_id='' THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;
  IF v_policy='' THEN RAISE EXCEPTION 'policy_version_id required' USING ERRCODE='22023'; END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN
    RAISE EXCEPTION 'append_evidence_refs must be a json array' USING ERRCODE='22023';
  END IF;

  -- confirm + approvals required
  IF p_confirm IS DISTINCT FROM true THEN
    RAISE EXCEPTION 'confirm=true required for apply' USING ERRCODE='22023';
  END IF;
  IF v_req IS NULL OR v_appr IS NULL THEN
    RAISE EXCEPTION 'requested_by and approved_by required for apply' USING ERRCODE='22023';
  END IF;

  PERFORM 1 FROM public.projects p WHERE p.id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  -- compute diff (must be clean for noop apply)
  v_diff := public.ledger_rebuild_run_dry_run_compute_v14(v_project_id, p_from_ts, p_to_ts);
  v_oob  := COALESCE((v_diff->>'out_of_balance_count')::int, 0);
  v_cur  := COALESCE((v_diff->>'currency_mismatch_count')::int, 0);
  v_miss := COALESCE((v_diff->>'missing_in_ledger_count')::int, 0);
  v_extra:= COALESCE((v_diff->>'extra_in_ledger_count')::int, 0);

  v_id := gen_random_uuid();

  INSERT INTO public.ledger_rebuild_runs(
    id, project_id, mode, from_ts, to_ts,
    status, requested_by, approved_by, confirm,
    run_id, trace_id, policy_version_id,
    diff_summary, evidence_refs
  )
  VALUES(
    v_id, v_project_id, 'apply', p_from_ts, p_to_ts,
    'running', v_req, v_appr, true,
    v_run_id, v_trace_id, v_policy,
    v_diff, p_append_evidence_refs
  );

  IF v_oob=0 AND v_cur=0 AND v_miss=0 AND v_extra=0 THEN
    UPDATE public.ledger_rebuild_runs
       SET status='succeeded',
           updated_at=now()
     WHERE id=v_id;

    rebuild_run_id := v_id;
    status := 'succeeded';
    diff_summary := v_diff;
    RETURN NEXT;
    RETURN;
  END IF;

  UPDATE public.ledger_rebuild_runs
     SET status='failed_recorded',
         updated_at=now()
   WHERE id=v_id;

  rebuild_run_id := v_id;
  status := 'failed_recorded';
  diff_summary := v_diff;
  RETURN NEXT;
END;
$$;

REVOKE ALL ON FUNCTION public.ledger_rebuild_apply_v14(
  text,timestamptz,timestamptz,boolean,text,text,text,text,text,jsonb
) FROM PUBLIC;

COMMIT;