-- 0121_v151_funds_to_ledger_posting_exec_only_fns.sql
-- v15.1 P0+1: Funds SoT -> v14 Ledger postings
--
-- Functions:
-- - refund_post_to_ledger_v151: refunds_v15(succeeded) -> ledger_postings (posting_key idempotent)
--   - debit:  platform:expense_refunds
--   - credit: provider:internal:clearing
-- - payout_post_to_ledger_v151: payouts_v15(completed) -> ledger_postings (posting_key idempotent)
--   - debit:  platform:expense_payouts
--   - credit: provider:internal:clearing
--
-- Design:
-- - No hard-throw for external/ledger failures: catch and mark review_required + open funds_operations.
-- - Confirm + policy_version_id mandatory (fail-closed).
-- - Uses v14 EXECUTE ONLY functions:
--   ledger_posting_create_v14, ledger_entries_insert_v14, ledger_posting_finalize_v14
--
-- Requires ledger_accounts to exist (active, same currency):
-- - platform:expense_refunds (expense)
-- - platform:expense_payouts (expense)
-- - provider:internal:clearing (asset or liability; your choice)
--
BEGIN;

-- -------------------------
-- helper: open funds op (best-effort; never throws)
-- -------------------------
CREATE OR REPLACE FUNCTION public._funds_op_open_best_effort_v151(
  p_project_id varchar,
  p_object_type text,
  p_object_key text,
  p_severity text,
  p_reason text,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  BEGIN
    PERFORM public.funds_op_open_v15(
      p_project_id,
      CASE
        WHEN p_object_type='refund' THEN 'refund_review'
        WHEN p_object_type='payout' THEN 'payout_review'
        ELSE 'settlement_review'
      END,
      p_object_type,
      p_object_key,
      p_severity,
      p_reason,
      p_run_id,
      p_trace_id,
      p_policy_version_id,
      p_evidence_refs
    );
  EXCEPTION WHEN OTHERS THEN
    -- swallow (best-effort)
    NULL;
  END;
END;
$$;

-- =========================================================
-- refunds_v15 -> ledger
-- =========================================================
CREATE OR REPLACE FUNCTION public.refund_post_to_ledger_v151(
  p_project_id varchar,
  p_refund_key text,
  p_confirm boolean,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(ledger_posting_id uuid, ledger_status text, posting_key char(64), action text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_refund_key text := btrim(coalesce(p_refund_key,''));
  v_run text := btrim(coalesce(p_run_id,''));
  v_trace text := btrim(coalesce(p_trace_id,''));
  v_policy text := btrim(coalesce(p_policy_version_id,''));

  v_currency text;
  v_amount bigint;
  v_posting_key char(64);
  v_source_event_key varchar(128);

  v_existing_posting_id uuid;

  v_create record;
  v_final record;

  v_entries jsonb;

  v_reason text;

BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_refund_key='' THEN RAISE EXCEPTION 'refund_key required' USING ERRCODE='22023'; END IF;
  IF p_confirm IS DISTINCT FROM true THEN RAISE EXCEPTION 'confirm=true required' USING ERRCODE='22023'; END IF;
  IF v_run='' OR v_trace='' OR v_policy='' THEN RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023'; END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN RAISE EXCEPTION 'append_evidence_refs must be array' USING ERRCODE='22023'; END IF;

  -- lock refund row
  SELECT
    r.currency, r.amount_minor, r.posting_key, r.source_event_key, r.ledger_posting_id, r.status
  INTO
    v_currency, v_amount, v_posting_key, v_source_event_key, v_existing_posting_id, v_reason
  FROM public.refunds_v15 r
  WHERE r.project_id=v_project_id::varchar(26) AND r.refund_key=v_refund_key
  FOR UPDATE;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'refund not found' USING ERRCODE='23503';
  END IF;

  IF v_reason <> 'succeeded' THEN
    -- Only succeeded can be posted (P0). Anything else => review_required
    PERFORM public._funds_op_open_best_effort_v151(
      v_project_id::varchar(26), 'refund', v_refund_key, 'high',
      'refund_post_to_ledger: status is not succeeded',
      v_run, v_trace, v_policy, p_append_evidence_refs
    );
    ledger_posting_id := NULL;
    ledger_status := 'skipped_not_succeeded';
    posting_key := v_posting_key;
    action := 'noop';
    RETURN NEXT;
    RETURN;
  END IF;

  IF v_existing_posting_id IS NOT NULL THEN
    ledger_posting_id := v_existing_posting_id;
    ledger_status := 'already_linked';
    posting_key := v_posting_key;
    action := 'noop';
    RETURN NEXT;
    RETURN;
  END IF;

  IF v_source_event_key IS NULL OR btrim(v_source_event_key)='' THEN
    v_source_event_key := ('v15:refund:' || v_refund_key)::varchar(128);
  END IF;

  -- Build entries (expense_refunds fixed)
  v_entries := jsonb_build_array(
    jsonb_build_object(
      'account_key','platform:expense_refunds',
      'direction','debit',
      'amount',v_amount,
      'currency',v_currency,
      'entry_key','line:1',
      'evidence_refs',p_append_evidence_refs
    ),
    jsonb_build_object(
      'account_key','provider:internal:clearing',
      'direction','credit',
      'amount',v_amount,
      'currency',v_currency,
      'entry_key','line:2',
      'evidence_refs',p_append_evidence_refs
    )
  );

  BEGIN
    -- create posting (idempotent by posting_key)
    SELECT * INTO v_create FROM public.ledger_posting_create_v14(
      v_project_id,
      v_posting_key::text,
      v_source_event_key::text,
      'refund'::ledger_posting_type_v14,
      v_currency,
      now(),
      v_run,
      v_trace,
      v_policy,
      p_append_evidence_refs
    );

    -- insert entries (strict)
    PERFORM public.ledger_entries_insert_v14(v_create.posting_id, v_entries);

    -- finalize (zero-sum)
    SELECT * INTO v_final FROM public.ledger_posting_finalize_v14(v_create.posting_id, p_append_evidence_refs);

    IF v_final.status::text = 'posted' THEN
      UPDATE public.refunds_v15 r
         SET ledger_posting_id = v_final.posting_id,
             evidence_refs = (COALESCE(r.evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
             run_id = v_run,
             trace_id = v_trace,
             policy_version_id = v_policy,
             updated_at = now()
       WHERE r.project_id=v_project_id::varchar(26) AND r.refund_key=v_refund_key;

      ledger_posting_id := v_final.posting_id;
      ledger_status := 'posted';
      posting_key := v_posting_key;
      action := CASE WHEN v_create.status='already_exists' THEN 'linked_existing' ELSE 'created_and_linked' END;
      RETURN NEXT;
      RETURN;
    END IF;

    -- finalize failed => review_required
    UPDATE public.refunds_v15 r
       SET status='review_required',
           failure_code='ledger_finalize_failed',
           failure_message=('ledger finalize status=' || v_final.status::text),
           evidence_refs=(COALESCE(r.evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
           run_id=v_run,
           trace_id=v_trace,
           policy_version_id=v_policy,
           updated_at=now()
     WHERE r.project_id=v_project_id::varchar(26) AND r.refund_key=v_refund_key;

    PERFORM public._funds_op_open_best_effort_v151(
      v_project_id::varchar(26), 'refund', v_refund_key, 'high',
      'refund_post_to_ledger: ledger finalize failed',
      v_run, v_trace, v_policy, p_append_evidence_refs
    );

    ledger_posting_id := v_final.posting_id;
    ledger_status := v_final.status::text;
    posting_key := v_posting_key;
    action := 'failed_recorded';
    RETURN NEXT;
    RETURN;

  EXCEPTION WHEN OTHERS THEN
    UPDATE public.refunds_v15 r
       SET status='review_required',
           failure_code='ledger_post_exception',
           failure_message=SQLERRM,
           evidence_refs=(COALESCE(r.evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
           run_id=v_run,
           trace_id=v_trace,
           policy_version_id=v_policy,
           updated_at=now()
     WHERE r.project_id=v_project_id::varchar(26) AND r.refund_key=v_refund_key;

    PERFORM public._funds_op_open_best_effort_v151(
      v_project_id::varchar(26), 'refund', v_refund_key, 'critical',
      ('refund_post_to_ledger exception: ' || SQLERRM),
      v_run, v_trace, v_policy, p_append_evidence_refs
    );

    ledger_posting_id := NULL;
    ledger_status := 'exception';
    posting_key := v_posting_key;
    action := 'failed_recorded';
    RETURN NEXT;
    RETURN;
  END;

END;
$$;

-- =========================================================
-- payouts_v15 -> ledger
-- =========================================================
CREATE OR REPLACE FUNCTION public.payout_post_to_ledger_v151(
  p_project_id varchar,
  p_payout_key text,
  p_confirm boolean,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(ledger_posting_id uuid, ledger_status text, posting_key char(64), action text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_payout_key text := btrim(coalesce(p_payout_key,''));
  v_run text := btrim(coalesce(p_run_id,''));
  v_trace text := btrim(coalesce(p_trace_id,''));
  v_policy text := btrim(coalesce(p_policy_version_id,''));

  v_currency text;
  v_amount bigint;
  v_posting_key char(64);
  v_source_event_key varchar(128);

  v_existing_posting_id uuid;

  v_create record;
  v_final record;

  v_entries jsonb;

  v_status text;

BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_payout_key='' THEN RAISE EXCEPTION 'payout_key required' USING ERRCODE='22023'; END IF;
  IF p_confirm IS DISTINCT FROM true THEN RAISE EXCEPTION 'confirm=true required' USING ERRCODE='22023'; END IF;
  IF v_run='' OR v_trace='' OR v_policy='' THEN RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023'; END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN RAISE EXCEPTION 'append_evidence_refs must be array' USING ERRCODE='22023'; END IF;

  SELECT
    p.currency, p.amount_net_minor, p.posting_key, p.source_event_key, p.ledger_posting_id, p.status
  INTO
    v_currency, v_amount, v_posting_key, v_source_event_key, v_existing_posting_id, v_status
  FROM public.payouts_v15 p
  WHERE p.project_id=v_project_id::varchar(26) AND p.payout_key=v_payout_key
  FOR UPDATE;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'payout not found' USING ERRCODE='23503';
  END IF;

  IF v_status <> 'completed' THEN
    PERFORM public._funds_op_open_best_effort_v151(
      v_project_id::varchar(26), 'payout', v_payout_key, 'high',
      'payout_post_to_ledger: status is not completed',
      v_run, v_trace, v_policy, p_append_evidence_refs
    );
    ledger_posting_id := NULL;
    ledger_status := 'skipped_not_completed';
    posting_key := v_posting_key;
    action := 'noop';
    RETURN NEXT;
    RETURN;
  END IF;

  IF v_existing_posting_id IS NOT NULL THEN
    ledger_posting_id := v_existing_posting_id;
    ledger_status := 'already_linked';
    posting_key := v_posting_key;
    action := 'noop';
    RETURN NEXT;
    RETURN;
  END IF;

  IF v_source_event_key IS NULL OR btrim(v_source_event_key)='' THEN
    v_source_event_key := ('v15:payout:' || v_payout_key)::varchar(128);
  END IF;

  -- P0: treat payout as expense (safe, later can move to payable model)
  v_entries := jsonb_build_array(
    jsonb_build_object(
      'account_key','platform:expense_payouts',
      'direction','debit',
      'amount',v_amount,
      'currency',v_currency,
      'entry_key','line:1',
      'evidence_refs',p_append_evidence_refs
    ),
    jsonb_build_object(
      'account_key','provider:internal:clearing',
      'direction','credit',
      'amount',v_amount,
      'currency',v_currency,
      'entry_key','line:2',
      'evidence_refs',p_append_evidence_refs
    )
  );

  BEGIN
    SELECT * INTO v_create FROM public.ledger_posting_create_v14(
      v_project_id,
      v_posting_key::text,
      v_source_event_key::text,
      'payout'::ledger_posting_type_v14,
      v_currency,
      now(),
      v_run,
      v_trace,
      v_policy,
      p_append_evidence_refs
    );

    PERFORM public.ledger_entries_insert_v14(v_create.posting_id, v_entries);

    SELECT * INTO v_final FROM public.ledger_posting_finalize_v14(v_create.posting_id, p_append_evidence_refs);

    IF v_final.status::text = 'posted' THEN
      UPDATE public.payouts_v15 p
         SET ledger_posting_id = v_final.posting_id,
             evidence_refs = (COALESCE(p.evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
             run_id = v_run,
             trace_id = v_trace,
             policy_version_id = v_policy,
             updated_at = now()
       WHERE p.project_id=v_project_id::varchar(26) AND p.payout_key=v_payout_key;

      ledger_posting_id := v_final.posting_id;
      ledger_status := 'posted';
      posting_key := v_posting_key;
      action := CASE WHEN v_create.status='already_exists' THEN 'linked_existing' ELSE 'created_and_linked' END;
      RETURN NEXT;
      RETURN;
    END IF;

    UPDATE public.payouts_v15 p
       SET status='review_required',
           failure_code='ledger_finalize_failed',
           failure_message=('ledger finalize status=' || v_final.status::text),
           evidence_refs=(COALESCE(p.evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
           run_id=v_run,
           trace_id=v_trace,
           policy_version_id=v_policy,
           updated_at=now()
     WHERE p.project_id=v_project_id::varchar(26) AND p.payout_key=v_payout_key;

    PERFORM public._funds_op_open_best_effort_v151(
      v_project_id::varchar(26), 'payout', v_payout_key, 'high',
      'payout_post_to_ledger: ledger finalize failed',
      v_run, v_trace, v_policy, p_append_evidence_refs
    );

    ledger_posting_id := v_final.posting_id;
    ledger_status := v_final.status::text;
    posting_key := v_posting_key;
    action := 'failed_recorded';
    RETURN NEXT;
    RETURN;

  EXCEPTION WHEN OTHERS THEN
    UPDATE public.payouts_v15 p
       SET status='review_required',
           failure_code='ledger_post_exception',
           failure_message=SQLERRM,
           evidence_refs=(COALESCE(p.evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
           run_id=v_run,
           trace_id=v_trace,
           policy_version_id=v_policy,
           updated_at=now()
     WHERE p.project_id=v_project_id::varchar(26) AND p.payout_key=v_payout_key;

    PERFORM public._funds_op_open_best_effort_v151(
      v_project_id::varchar(26), 'payout', v_payout_key, 'critical',
      ('payout_post_to_ledger exception: ' || SQLERRM),
      v_run, v_trace, v_policy, p_append_evidence_refs
    );

    ledger_posting_id := NULL;
    ledger_status := 'exception';
    posting_key := v_posting_key;
    action := 'failed_recorded';
    RETURN NEXT;
    RETURN;
  END;

END;
$$;

-- =========================
-- SECURITY: revoke from PUBLIC
-- =========================
REVOKE ALL ON FUNCTION public._funds_op_open_best_effort_v151(varchar,text,text,text,text,text,text,text,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.refund_post_to_ledger_v151(varchar,text,boolean,text,text,text,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.payout_post_to_ledger_v151(varchar,text,boolean,text,text,text,jsonb) FROM PUBLIC;

COMMIT;