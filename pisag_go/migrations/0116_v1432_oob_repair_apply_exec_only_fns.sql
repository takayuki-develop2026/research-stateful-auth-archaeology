-- 0116_v1432_oob_repair_apply_exec_only_fns.sql
-- v14.3.2: out_of_balance repair apply (failed_recorded postings only)
-- Strategy:
-- - Only repair postings with status='failed_recorded' and debit!=credit within window
-- - Add one balancing entry to a configured suspense account_key (same currency)
-- - Finalize posting to posted
-- - Record an apply run in ledger_rebuild_runs (mode=apply)

BEGIN;

CREATE OR REPLACE FUNCTION public.ledger_rebuild_apply_oob_repair_v1432(
  p_project_id text,
  p_from_ts timestamptz,
  p_to_ts timestamptz,
  p_confirm boolean,
  p_requested_by text,
  p_approved_by text,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_suspense_account_key text DEFAULT 'platform:suspense:oob',
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(rebuild_run_id uuid, status text, summary jsonb)
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
  v_suspense text := NULLIF(btrim(coalesce(p_suspense_account_key,'')), '');

  v_apply_id uuid := gen_random_uuid();

  v_suspense_id uuid;

  v_total int := 0;
  v_repaired int := 0;
  v_skipped_already_balanced int := 0;
  v_failed int := 0;

  rec record;

  v_debit bigint;
  v_credit bigint;
  v_delta bigint;

  v_dir text;
  v_amt bigint;
  v_entry_json jsonb;

  v_final record;

BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF p_from_ts IS NULL OR p_to_ts IS NULL OR NOT (p_from_ts < p_to_ts) THEN
    RAISE EXCEPTION 'valid from/to required' USING ERRCODE='22023'; END IF;
  IF v_run_id='' THEN RAISE EXCEPTION 'run_id required' USING ERRCODE='22023'; END IF;
  IF v_trace_id='' THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;
  IF v_policy='' THEN RAISE EXCEPTION 'policy_version_id required' USING ERRCODE='22023'; END IF;
  IF v_req IS NULL OR v_appr IS NULL THEN
    RAISE EXCEPTION 'requested_by and approved_by required' USING ERRCODE='22023';
  END IF;
  IF p_confirm IS DISTINCT FROM true THEN
    RAISE EXCEPTION 'confirm=true required' USING ERRCODE='22023';
  END IF;
  IF v_suspense IS NULL THEN
    RAISE EXCEPTION 'suspense_account_key required' USING ERRCODE='22023';
  END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN
    RAISE EXCEPTION 'append_evidence_refs must be a json array' USING ERRCODE='22023';
  END IF;

  PERFORM 1 FROM public.projects p WHERE p.id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  -- Resolve suspense account (must exist and active; currency is validated per posting later)
  SELECT id INTO v_suspense_id
  FROM public.ledger_accounts
  WHERE project_id=v_project_id AND account_key=v_suspense AND status='active'
  LIMIT 1;

  IF v_suspense_id IS NULL THEN
    RAISE EXCEPTION 'suspense account not found or inactive: %', v_suspense USING ERRCODE='22023';
  END IF;

  -- Create apply run row (running)
  INSERT INTO public.ledger_rebuild_runs(
    id, project_id, mode, from_ts, to_ts,
    status, requested_by, approved_by, confirm,
    run_id, trace_id, policy_version_id,
    diff_summary, evidence_refs
  )
  VALUES(
    v_apply_id, v_project_id, 'apply', p_from_ts, p_to_ts,
    'running', v_req, v_appr, true,
    v_run_id, v_trace_id, v_policy,
    '{}'::jsonb, p_append_evidence_refs
  );

  -- Iterate failed_recorded postings in window (posted_at axis)
  FOR rec IN
    SELECT p.id AS posting_id, p.posting_key, p.currency, p.posted_at
    FROM public.ledger_postings p
    WHERE p.project_id=v_project_id
      AND p.posted_at >= p_from_ts
      AND p.posted_at <  p_to_ts
      AND p.status = 'failed_recorded'
    ORDER BY p.posted_at ASC, p.id ASC
  LOOP
    v_total := v_total + 1;

    SELECT COALESCE(SUM(CASE WHEN e.direction='debit' THEN e.amount ELSE 0 END),0)::bigint
      INTO v_debit
    FROM public.ledger_entries e
    WHERE e.posting_id = rec.posting_id;

    SELECT COALESCE(SUM(CASE WHEN e.direction='credit' THEN e.amount ELSE 0 END),0)::bigint
      INTO v_credit
    FROM public.ledger_entries e
    WHERE e.posting_id = rec.posting_id;

    IF v_debit = v_credit THEN
      v_skipped_already_balanced := v_skipped_already_balanced + 1;
      CONTINUE;
    END IF;

    v_delta := v_debit - v_credit;

    -- v_delta > 0 means credit short; add credit to suspense
    IF v_delta > 0 THEN
      v_dir := 'credit';
      v_amt := v_delta;
    ELSE
      v_dir := 'debit';
      v_amt := -v_delta;
    END IF;

    -- Insert a single balancing entry (entry_key fixed; ON CONFLICT DO NOTHING in ledger_entries_insert_v14)
    v_entry_json := jsonb_build_array(
      jsonb_build_object(
        'account_key', v_suspense,
        'direction', v_dir,
        'amount', v_amt,
        'currency', rec.currency,
        'entry_key', 'repair:oob',
        'evidence_refs', p_append_evidence_refs
      )
    );

    BEGIN
      PERFORM public.ledger_entries_insert_v14(rec.posting_id, v_entry_json);
      SELECT * INTO v_final FROM public.ledger_posting_finalize_v14(rec.posting_id, p_append_evidence_refs);

      IF v_final.status::text = 'posted' THEN
        v_repaired := v_repaired + 1;
      ELSE
        v_failed := v_failed + 1;
      END IF;
    EXCEPTION WHEN OTHERS THEN
      v_failed := v_failed + 1;
      -- continue; do not throw
    END;
  END LOOP;

  summary := jsonb_build_object(
    'window', jsonb_build_object('from', p_from_ts, 'to', p_to_ts),
    'target_failed_recorded_count', v_total,
    'repaired_count', v_repaired,
    'skipped_already_balanced', v_skipped_already_balanced,
    'failed_count', v_failed,
    'suspense_account_key', v_suspense
  );

  IF v_failed = 0 THEN
    UPDATE public.ledger_rebuild_runs
       SET status='succeeded',
           diff_summary=summary,
           evidence_refs=(COALESCE(evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
           updated_at=now()
     WHERE id=v_apply_id;

    rebuild_run_id := v_apply_id;
    status := 'succeeded';
    RETURN NEXT;
    RETURN;
  END IF;

  UPDATE public.ledger_rebuild_runs
     SET status='failed_recorded',
         diff_summary=summary,
         evidence_refs=(COALESCE(evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
         updated_at=now()
   WHERE id=v_apply_id;

  rebuild_run_id := v_apply_id;
  status := 'failed_recorded';
  RETURN NEXT;
END;
$$;

REVOKE ALL ON FUNCTION public.ledger_rebuild_apply_oob_repair_v1432(
  text,timestamptz,timestamptz,boolean,text,text,text,text,text,text,jsonb
) FROM PUBLIC;

COMMIT;