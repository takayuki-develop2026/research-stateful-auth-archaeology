-- 0119_v15_funds_exec_only_fns.sql (FIXED)
-- v15 exec-only functions (P0):
-- - funds_op_open_v15 / funds_op_resolve_v15
-- - refund_create_requested_v15
-- - refund_mark_succeeded_from_utl_v15 / refund_mark_failed_v15
-- - payout_schedule_v15 / payout_mark_completed_from_utl_v15 / payout_mark_failed_v15
-- - hold_create_v15 / hold_release_v15
-- - dispute_upsert_from_utl_v15 / dispute_event_insert_v15
-- - settlement_batch_create_v15 / settlement_reconcile_dry_run_v15
--
-- FIXES:
-- - Avoid PL/pgSQL OUT param name collisions by using:
--   ON CONFLICT ON CONSTRAINT <constraint_name>
-- - Keep fail-closed validation (json array, confirm, required fields)

BEGIN;

-- We reuse:
--  - public._jsonb_is_array_v14(jsonb)
--  - public.sha256_hex_v14(text)

-- =========================
-- funds operations
-- =========================
CREATE OR REPLACE FUNCTION public.funds_op_open_v15(
  p_project_id varchar,
  p_op_type text,
  p_object_type text,
  p_object_key text,
  p_severity text,
  p_reason text,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(op_id uuid, op_key char(64), status text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_op_type text := btrim(coalesce(p_op_type,''));
  v_obj_type text := btrim(coalesce(p_object_type,''));
  v_obj_key text := btrim(coalesce(p_object_key,''));
  v_sev text := btrim(coalesce(p_severity,''));
  v_reason text := btrim(coalesce(p_reason,''));
  v_run text := btrim(coalesce(p_run_id,''));
  v_trace text := btrim(coalesce(p_trace_id,''));
  v_policy text := btrim(coalesce(p_policy_version_id,''));
  v_key char(64);
  v_id uuid;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_op_type='' OR v_obj_type='' OR v_obj_key='' THEN RAISE EXCEPTION 'op_type/object_type/object_key required' USING ERRCODE='22023'; END IF;
  IF v_sev='' THEN RAISE EXCEPTION 'severity required' USING ERRCODE='22023'; END IF;
  IF v_reason='' THEN RAISE EXCEPTION 'reason required' USING ERRCODE='22023'; END IF;
  IF v_run='' OR v_trace='' OR v_policy='' THEN RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023'; END IF;
  IF NOT public._jsonb_is_array_v14(p_evidence_refs) THEN RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023'; END IF;

  v_key := left(public.sha256_hex_v14(v_project_id||'|'||v_op_type||'|'||v_obj_type||'|'||v_obj_key||'|'||v_reason),64)::char(64);

  INSERT INTO public.funds_operations_v15(
    project_id, op_key, op_type, object_type, object_key,
    severity, status, reason, evidence_refs,
    run_id, trace_id, policy_version_id
  )
  VALUES (
    v_project_id::varchar(26), v_key, v_op_type, v_obj_type, v_obj_key,
    v_sev, 'open', v_reason, p_evidence_refs,
    v_run, v_trace, v_policy
  )
  ON CONFLICT ON CONSTRAINT uq_fo_v15_project_op_key DO UPDATE SET
    status='open',
    severity=EXCLUDED.severity,
    evidence_refs=(COALESCE(public.funds_operations_v15.evidence_refs,'[]'::jsonb) || EXCLUDED.evidence_refs),
    run_id=EXCLUDED.run_id,
    trace_id=EXCLUDED.trace_id,
    policy_version_id=EXCLUDED.policy_version_id,
    updated_at=now()
  RETURNING id INTO v_id;

  op_id := v_id;
  op_key := v_key;
  status := 'open';
  RETURN NEXT;
END;
$$;

CREATE OR REPLACE FUNCTION public.funds_op_resolve_v15(
  p_op_id uuid,
  p_new_status text, -- acknowledged|resolved|suppressed
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_new text := btrim(coalesce(p_new_status,''));
BEGIN
  IF p_op_id IS NULL THEN RAISE EXCEPTION 'op_id required' USING ERRCODE='22023'; END IF;
  IF v_new NOT IN ('acknowledged','resolved','suppressed') THEN RAISE EXCEPTION 'new_status invalid' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_run_id,''))='' OR btrim(coalesce(p_trace_id,''))='' OR btrim(coalesce(p_policy_version_id,''))='' THEN
    RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023';
  END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023'; END IF;

  UPDATE public.funds_operations_v15 fo
     SET status=v_new,
         evidence_refs=(COALESCE(fo.evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
         run_id=p_run_id,
         trace_id=p_trace_id,
         policy_version_id=p_policy_version_id,
         updated_at=now()
   WHERE fo.id=p_op_id;
END;
$$;

-- =========================
-- refunds
-- =========================
CREATE OR REPLACE FUNCTION public.refund_create_requested_v15(
  p_project_id varchar,
  p_refund_key text,             -- optional; if empty, generated internal:<uuid>
  p_provider text,
  p_shop_id text,
  p_currency text,
  p_amount_minor bigint,
  p_posting_key char(64),
  p_confirm boolean,
  p_idempotency_key text,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(refund_id uuid, refund_key text, status text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_refund_key text := btrim(coalesce(p_refund_key,''));
  v_provider text := btrim(coalesce(p_provider,''));
  v_shop text := btrim(coalesce(p_shop_id,''));
  v_ccy text := btrim(coalesce(p_currency,''));
  v_posting char(64) := p_posting_key;
  v_idem text := btrim(coalesce(p_idempotency_key,''));
  v_run text := btrim(coalesce(p_run_id,''));
  v_trace text := btrim(coalesce(p_trace_id,''));
  v_policy text := btrim(coalesce(p_policy_version_id,''));
  v_id uuid;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_provider='' THEN RAISE EXCEPTION 'provider required' USING ERRCODE='22023'; END IF;
  IF v_shop='' THEN RAISE EXCEPTION 'shop_id required' USING ERRCODE='22023'; END IF;
  IF v_ccy='' THEN RAISE EXCEPTION 'currency required' USING ERRCODE='22023'; END IF;
  IF p_amount_minor IS NULL OR p_amount_minor<=0 THEN RAISE EXCEPTION 'amount_minor must be >0' USING ERRCODE='22023'; END IF;
  IF v_posting IS NULL OR length(v_posting)<>64 THEN RAISE EXCEPTION 'posting_key must be 64 chars' USING ERRCODE='22023'; END IF;
  IF p_confirm IS DISTINCT FROM true THEN RAISE EXCEPTION 'confirm=true required' USING ERRCODE='22023'; END IF;
  IF v_idem='' THEN RAISE EXCEPTION 'idempotency_key required' USING ERRCODE='22023'; END IF;
  IF v_run='' OR v_trace='' OR v_policy='' THEN RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023'; END IF;
  IF NOT public._jsonb_is_array_v14(p_evidence_refs) THEN RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects p WHERE p.id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  v_id := (substring(public.sha256_hex_v14(v_project_id||'|refund_create|'||v_idem),1,32))::uuid;
  IF v_refund_key='' THEN
    v_refund_key := 'internal:' || v_id::text;
  END IF;

  INSERT INTO public.refunds_v15(
    id, project_id, refund_key, provider, shop_id, currency, amount_minor,
    status, requested_at, posting_key,
    evidence_refs, run_id, trace_id, policy_version_id
  )
  VALUES(
    v_id, v_project_id::varchar(26), v_refund_key, v_provider, v_shop, v_ccy, p_amount_minor,
    'requested', now(), v_posting,
    p_evidence_refs, v_run, v_trace, v_policy
  )
  ON CONFLICT ON CONSTRAINT uq_refunds_v15_project_refund_key DO UPDATE SET
    evidence_refs=(COALESCE(public.refunds_v15.evidence_refs,'[]'::jsonb) || EXCLUDED.evidence_refs),
    run_id=EXCLUDED.run_id,
    trace_id=EXCLUDED.trace_id,
    policy_version_id=EXCLUDED.policy_version_id,
    updated_at=now();

  refund_id := v_id;
  refund_key := v_refund_key;
  status := 'requested';
  RETURN NEXT;
END;
$$;

CREATE OR REPLACE FUNCTION public.refund_mark_succeeded_from_utl_v15(
  p_project_id varchar,
  p_refund_key text,
  p_source_event_key varchar(128),
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF btrim(coalesce(p_project_id::text,''))='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_refund_key,''))='' THEN RAISE EXCEPTION 'refund_key required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_run_id,''))='' OR btrim(coalesce(p_trace_id,''))='' OR btrim(coalesce(p_policy_version_id,''))='' THEN
    RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023';
  END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023'; END IF;

  UPDATE public.refunds_v15 r
     SET status='succeeded',
         source_event_key=p_source_event_key,
         settled_at=COALESCE(r.settled_at, now()),
         evidence_refs=(COALESCE(r.evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
         run_id=p_run_id,
         trace_id=p_trace_id,
         policy_version_id=p_policy_version_id,
         updated_at=now()
   WHERE r.project_id=p_project_id::varchar(26) AND r.refund_key=p_refund_key;
END;
$$;

CREATE OR REPLACE FUNCTION public.refund_mark_failed_v15(
  p_project_id varchar,
  p_refund_key text,
  p_failure_code text,
  p_failure_message text,
  p_retryable boolean,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_status text;
BEGIN
  IF btrim(coalesce(p_project_id::text,''))='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_refund_key,''))='' THEN RAISE EXCEPTION 'refund_key required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_run_id,''))='' OR btrim(coalesce(p_trace_id,''))='' OR btrim(coalesce(p_policy_version_id,''))='' THEN
    RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023';
  END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023'; END IF;

  v_status := CASE WHEN p_retryable IS TRUE THEN 'retryable' ELSE 'failed' END;

  UPDATE public.refunds_v15 r
     SET status=v_status,
         failed_at=now(),
         failure_code=p_failure_code,
         failure_message=p_failure_message,
         evidence_refs=(COALESCE(r.evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
         run_id=p_run_id,
         trace_id=p_trace_id,
         policy_version_id=p_policy_version_id,
         updated_at=now()
   WHERE r.project_id=p_project_id::varchar(26) AND r.refund_key=p_refund_key;
END;
$$;

-- =========================
-- payouts
-- =========================
CREATE OR REPLACE FUNCTION public.payout_schedule_v15(
  p_project_id varchar,
  p_payout_key text,             -- optional; if empty, generated internal:<uuid>
  p_provider text,
  p_shop_id text,
  p_currency text,
  p_amount_net_minor bigint,
  p_posting_key char(64),
  p_scheduled_for date,
  p_confirm boolean,
  p_idempotency_key text,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(payout_id uuid, payout_key text, status text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_payout_key text := btrim(coalesce(p_payout_key,''));
  v_provider text := btrim(coalesce(p_provider,''));
  v_shop text := btrim(coalesce(p_shop_id,''));
  v_ccy text := btrim(coalesce(p_currency,''));
  v_posting char(64) := p_posting_key;
  v_idem text := btrim(coalesce(p_idempotency_key,''));
  v_run text := btrim(coalesce(p_run_id,''));
  v_trace text := btrim(coalesce(p_trace_id,''));
  v_policy text := btrim(coalesce(p_policy_version_id,''));
  v_id uuid;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_provider='' THEN RAISE EXCEPTION 'provider required' USING ERRCODE='22023'; END IF;
  IF v_shop='' THEN RAISE EXCEPTION 'shop_id required' USING ERRCODE='22023'; END IF;
  IF v_ccy='' THEN RAISE EXCEPTION 'currency required' USING ERRCODE='22023'; END IF;
  IF p_amount_net_minor IS NULL OR p_amount_net_minor<=0 THEN RAISE EXCEPTION 'amount_net_minor must be >0' USING ERRCODE='22023'; END IF;
  IF v_posting IS NULL OR length(v_posting)<>64 THEN RAISE EXCEPTION 'posting_key must be 64 chars' USING ERRCODE='22023'; END IF;
  IF p_scheduled_for IS NULL THEN RAISE EXCEPTION 'scheduled_for required' USING ERRCODE='22023'; END IF;
  IF p_confirm IS DISTINCT FROM true THEN RAISE EXCEPTION 'confirm=true required' USING ERRCODE='22023'; END IF;
  IF v_idem='' THEN RAISE EXCEPTION 'idempotency_key required' USING ERRCODE='22023'; END IF;
  IF v_run='' OR v_trace='' OR v_policy='' THEN RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023'; END IF;
  IF NOT public._jsonb_is_array_v14(p_evidence_refs) THEN RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects p WHERE p.id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  v_id := (substring(public.sha256_hex_v14(v_project_id||'|payout_schedule|'||v_idem),1,32))::uuid;
  IF v_payout_key='' THEN
    v_payout_key := 'internal:' || v_id::text;
  END IF;

  INSERT INTO public.payouts_v15(
    id, project_id, payout_key, provider, shop_id, currency,
    amount_gross_minor, amount_net_minor,
    status, scheduled_for, posting_key,
    evidence_refs, run_id, trace_id, policy_version_id
  )
  VALUES(
    v_id, v_project_id::varchar(26), v_payout_key, v_provider, v_shop, v_ccy,
    p_amount_net_minor, p_amount_net_minor,
    'scheduled', p_scheduled_for, v_posting,
    p_evidence_refs, v_run, v_trace, v_policy
  )
  ON CONFLICT ON CONSTRAINT uq_payouts_v15_project_payout_key DO UPDATE SET
    evidence_refs=(COALESCE(public.payouts_v15.evidence_refs,'[]'::jsonb) || EXCLUDED.evidence_refs),
    run_id=EXCLUDED.run_id,
    trace_id=EXCLUDED.trace_id,
    policy_version_id=EXCLUDED.policy_version_id,
    updated_at=now();

  payout_id := v_id;
  payout_key := v_payout_key;
  status := 'scheduled';
  RETURN NEXT;
END;
$$;

CREATE OR REPLACE FUNCTION public.payout_mark_completed_from_utl_v15(
  p_project_id varchar,
  p_payout_key text,
  p_source_event_key varchar(128),
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF btrim(coalesce(p_project_id::text,''))='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_payout_key,''))='' THEN RAISE EXCEPTION 'payout_key required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_run_id,''))='' OR btrim(coalesce(p_trace_id,''))='' OR btrim(coalesce(p_policy_version_id,''))='' THEN
    RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023';
  END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023'; END IF;

  UPDATE public.payouts_v15 p
     SET status='completed',
         source_event_key=p_source_event_key,
         completed_at=COALESCE(p.completed_at, now()),
         evidence_refs=(COALESCE(p.evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
         run_id=p_run_id,
         trace_id=p_trace_id,
         policy_version_id=p_policy_version_id,
         updated_at=now()
   WHERE p.project_id=p_project_id::varchar(26) AND p.payout_key=p_payout_key;
END;
$$;

CREATE OR REPLACE FUNCTION public.payout_mark_failed_v15(
  p_project_id varchar,
  p_payout_key text,
  p_failure_code text,
  p_failure_message text,
  p_retryable boolean,
  p_next_retry_at timestamptz,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_status text;
BEGIN
  IF btrim(coalesce(p_project_id::text,''))='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_payout_key,''))='' THEN RAISE EXCEPTION 'payout_key required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_run_id,''))='' OR btrim(coalesce(p_trace_id,''))='' OR btrim(coalesce(p_policy_version_id,''))='' THEN
    RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023';
  END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023'; END IF;

  v_status := CASE WHEN p_retryable IS TRUE THEN 'retryable' ELSE 'failed' END;

  UPDATE public.payouts_v15 p
     SET status=v_status,
         failed_at=now(),
         failure_code=p_failure_code,
         failure_message=p_failure_message,
         attempt_count=p.attempt_count+1,
         next_retry_at=p_next_retry_at,
         evidence_refs=(COALESCE(p.evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
         run_id=p_run_id,
         trace_id=p_trace_id,
         policy_version_id=p_policy_version_id,
         updated_at=now()
   WHERE p.project_id=p_project_id::varchar(26) AND p.payout_key=p_payout_key;
END;
$$;

-- =========================
-- holds
-- =========================
CREATE OR REPLACE FUNCTION public.hold_create_v15(
  p_project_id varchar,
  p_hold_key text,           -- optional internal:<uuid>
  p_hold_type text,
  p_scope_type text,
  p_scope_id text,
  p_currency text,
  p_amount_minor bigint,
  p_reason text,
  p_expires_at timestamptz,
  p_confirm boolean,
  p_idempotency_key text,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(hold_id uuid, hold_key text, status text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_hold_key text := btrim(coalesce(p_hold_key,''));
  v_idem text := btrim(coalesce(p_idempotency_key,''));
  v_run text := btrim(coalesce(p_run_id,''));
  v_trace text := btrim(coalesce(p_trace_id,''));
  v_policy text := btrim(coalesce(p_policy_version_id,''));
  v_id uuid;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_hold_type,''))='' OR btrim(coalesce(p_scope_type,''))='' THEN RAISE EXCEPTION 'hold_type/scope_type required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_currency,''))='' THEN RAISE EXCEPTION 'currency required' USING ERRCODE='22023'; END IF;
  IF p_amount_minor IS NULL OR p_amount_minor<=0 THEN RAISE EXCEPTION 'amount_minor must be >0' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_reason,''))='' THEN RAISE EXCEPTION 'reason required' USING ERRCODE='22023'; END IF;
  IF p_confirm IS DISTINCT FROM true THEN RAISE EXCEPTION 'confirm=true required' USING ERRCODE='22023'; END IF;
  IF v_idem='' THEN RAISE EXCEPTION 'idempotency_key required' USING ERRCODE='22023'; END IF;
  IF v_run='' OR v_trace='' OR v_policy='' THEN RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023'; END IF;
  IF NOT public._jsonb_is_array_v14(p_evidence_refs) THEN RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023'; END IF;

  v_id := (substring(public.sha256_hex_v14(v_project_id||'|hold_create|'||v_idem),1,32))::uuid;
  IF v_hold_key='' THEN
    v_hold_key := 'internal:' || v_id::text;
  END IF;

  INSERT INTO public.holds_v15(
    id, project_id, hold_key, hold_type, scope_type, scope_id,
    currency, amount_minor, status, reason, expires_at,
    evidence_refs, run_id, trace_id, policy_version_id
  )
  VALUES(
    v_id, v_project_id::varchar(26), v_hold_key, p_hold_type, p_scope_type, NULLIF(btrim(coalesce(p_scope_id,'')),''),
    p_currency, p_amount_minor, 'active', p_reason, p_expires_at,
    p_evidence_refs, v_run, v_trace, v_policy
  )
  ON CONFLICT ON CONSTRAINT uq_holds_v15_project_hold_key DO UPDATE SET
    status='active',
    evidence_refs=(COALESCE(public.holds_v15.evidence_refs,'[]'::jsonb) || EXCLUDED.evidence_refs),
    run_id=EXCLUDED.run_id,
    trace_id=EXCLUDED.trace_id,
    policy_version_id=EXCLUDED.policy_version_id,
    updated_at=now();

  hold_id := v_id;
  hold_key := v_hold_key;
  status := 'active';
  RETURN NEXT;
END;
$$;

CREATE OR REPLACE FUNCTION public.hold_release_v15(
  p_project_id varchar,
  p_hold_key text,
  p_confirm boolean,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF btrim(coalesce(p_project_id::text,''))='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_hold_key,''))='' THEN RAISE EXCEPTION 'hold_key required' USING ERRCODE='22023'; END IF;
  IF p_confirm IS DISTINCT FROM true THEN RAISE EXCEPTION 'confirm=true required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_run_id,''))='' OR btrim(coalesce(p_trace_id,''))='' OR btrim(coalesce(p_policy_version_id,''))='' THEN
    RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023';
  END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023'; END IF;

  UPDATE public.holds_v15 h
     SET status='released',
         evidence_refs=(COALESCE(h.evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
         run_id=p_run_id,
         trace_id=p_trace_id,
         policy_version_id=p_policy_version_id,
         updated_at=now()
   WHERE h.project_id=p_project_id::varchar(26) AND h.hold_key=p_hold_key AND h.status='active';
END;
$$;

-- =========================
-- disputes (upsert + event insert)
-- =========================
CREATE OR REPLACE FUNCTION public.dispute_upsert_from_utl_v15(
  p_project_id varchar,
  p_dispute_key text,
  p_provider text,
  p_currency text,
  p_amount_minor bigint,
  p_status text,
  p_opened_at timestamptz,
  p_due_by timestamptz,
  p_source_event_key varchar(128),
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(dispute_id uuid, status text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_key text := btrim(coalesce(p_dispute_key,''));
  v_provider text := btrim(coalesce(p_provider,''));
  v_ccy text := btrim(coalesce(p_currency,''));
  v_st text := btrim(coalesce(p_status,''));
  v_run text := btrim(coalesce(p_run_id,''));
  v_trace text := btrim(coalesce(p_trace_id,''));
  v_policy text := btrim(coalesce(p_policy_version_id,''));
  v_id uuid;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_key='' THEN RAISE EXCEPTION 'dispute_key required' USING ERRCODE='22023'; END IF;
  IF v_provider='' THEN RAISE EXCEPTION 'provider required' USING ERRCODE='22023'; END IF;
  IF v_ccy='' THEN RAISE EXCEPTION 'currency required' USING ERRCODE='22023'; END IF;
  IF p_amount_minor IS NULL OR p_amount_minor<=0 THEN RAISE EXCEPTION 'amount_minor must be >0' USING ERRCODE='22023'; END IF;
  IF v_st NOT IN ('opened','evidence_required','under_review','won','lost','closed','review_required') THEN
    RAISE EXCEPTION 'status invalid' USING ERRCODE='22023';
  END IF;
  IF p_opened_at IS NULL THEN RAISE EXCEPTION 'opened_at required' USING ERRCODE='22023'; END IF;
  IF v_run='' OR v_trace='' OR v_policy='' THEN RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023'; END IF;
  IF NOT public._jsonb_is_array_v14(p_evidence_refs) THEN RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023'; END IF;

  v_id := (substring(public.sha256_hex_v14(v_project_id||'|dispute|'||v_key),1,32))::uuid;

  INSERT INTO public.disputes_v15(
    id, project_id, dispute_key, provider, currency, amount_minor,
    status, opened_at, due_by, source_event_key, evidence_refs,
    run_id, trace_id, policy_version_id
  )
  VALUES(
    v_id, v_project_id::varchar(26), v_key, v_provider, v_ccy, p_amount_minor,
    v_st, p_opened_at, p_due_by, p_source_event_key, p_evidence_refs,
    v_run, v_trace, v_policy
  )
  ON CONFLICT ON CONSTRAINT uq_disputes_v15_project_dispute_key DO UPDATE SET
    status=EXCLUDED.status,
    due_by=EXCLUDED.due_by,
    source_event_key=COALESCE(EXCLUDED.source_event_key, public.disputes_v15.source_event_key),
    evidence_refs=(COALESCE(public.disputes_v15.evidence_refs,'[]'::jsonb) || EXCLUDED.evidence_refs),
    run_id=EXCLUDED.run_id,
    trace_id=EXCLUDED.trace_id,
    policy_version_id=EXCLUDED.policy_version_id,
    updated_at=now()
  RETURNING id INTO v_id;

  dispute_id := v_id;
  status := v_st;
  RETURN NEXT;
END;
$$;

CREATE OR REPLACE FUNCTION public.dispute_event_insert_v15(
  p_project_id varchar,
  p_dispute_key text,
  p_event_key varchar(128),
  p_event_type text,
  p_occurred_at timestamptz,
  p_payload_evidence_ref uuid,
  p_run_id text,
  p_trace_id text
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_key text := btrim(coalesce(p_dispute_key,''));
  v_event_key text := btrim(coalesce(p_event_key::text,''));
  v_type text := btrim(coalesce(p_event_type,''));
  v_run text := btrim(coalesce(p_run_id,''));
  v_trace text := btrim(coalesce(p_trace_id,''));
  v_dispute_id uuid;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_key='' THEN RAISE EXCEPTION 'dispute_key required' USING ERRCODE='22023'; END IF;
  IF v_event_key='' THEN RAISE EXCEPTION 'event_key required' USING ERRCODE='22023'; END IF;
  IF v_type='' THEN RAISE EXCEPTION 'event_type required' USING ERRCODE='22023'; END IF;
  IF p_occurred_at IS NULL THEN RAISE EXCEPTION 'occurred_at required' USING ERRCODE='22023'; END IF;
  IF v_run='' OR v_trace='' THEN RAISE EXCEPTION 'run_id/trace_id required' USING ERRCODE='22023'; END IF;

  SELECT d.id INTO v_dispute_id
  FROM public.disputes_v15 d
  WHERE d.project_id=v_project_id::varchar(26) AND d.dispute_key=v_key
  LIMIT 1;

  IF v_dispute_id IS NULL THEN
    RAISE EXCEPTION 'dispute not found for dispute_key' USING ERRCODE='23503';
  END IF;

  INSERT INTO public.dispute_events_v15(
    project_id, dispute_id, event_key, event_type, occurred_at,
    payload_evidence_ref, run_id, trace_id
  )
  VALUES(
    v_project_id::varchar(26), v_dispute_id, v_event_key::varchar(128), v_type, p_occurred_at,
    p_payload_evidence_ref, v_run, v_trace
  )
  ON CONFLICT ON CONSTRAINT uq_dispute_events_v15_dispute_event_key DO NOTHING;
END;
$$;

-- =========================
-- settlement batch + reconcile dry_run (items_json)
-- =========================
CREATE OR REPLACE FUNCTION public.settlement_batch_create_v15(
  p_project_id varchar,
  p_provider text,
  p_batch_key text,
  p_from_at timestamptz,
  p_to_at timestamptz,
  p_artifact_ref text,
  p_confirm boolean,
  p_idempotency_key text,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(batch_id uuid, status text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_provider text := btrim(coalesce(p_provider,''));
  v_key text := btrim(coalesce(p_batch_key,''));
  v_idem text := btrim(coalesce(p_idempotency_key,''));
  v_run text := btrim(coalesce(p_run_id,''));
  v_trace text := btrim(coalesce(p_trace_id,''));
  v_policy text := btrim(coalesce(p_policy_version_id,''));
  v_id uuid;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_provider='' THEN RAISE EXCEPTION 'provider required' USING ERRCODE='22023'; END IF;
  IF v_key='' THEN RAISE EXCEPTION 'batch_key required' USING ERRCODE='22023'; END IF;
  IF p_from_at IS NULL OR p_to_at IS NULL OR NOT (p_from_at < p_to_at) THEN RAISE EXCEPTION 'valid from/to required' USING ERRCODE='22023'; END IF;
  IF p_confirm IS DISTINCT FROM true THEN RAISE EXCEPTION 'confirm=true required' USING ERRCODE='22023'; END IF;
  IF v_idem='' THEN RAISE EXCEPTION 'idempotency_key required' USING ERRCODE='22023'; END IF;
  IF v_run='' OR v_trace='' OR v_policy='' THEN RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023'; END IF;
  IF NOT public._jsonb_is_array_v14(p_evidence_refs) THEN RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023'; END IF;

  v_id := (substring(public.sha256_hex_v14(v_project_id||'|settlement_batch_create|'||v_idem),1,32))::uuid;

  INSERT INTO public.settlement_batches_v15(
    id, project_id, provider, batch_key, status,
    from_at, to_at, artifact_ref,
    run_id, trace_id, policy_version_id, evidence_refs
  )
  VALUES(
    v_id, v_project_id::varchar(26), v_provider, v_key, 'open',
    p_from_at, p_to_at, NULLIF(btrim(coalesce(p_artifact_ref,'')),''),
    v_run, v_trace, v_policy, p_evidence_refs
  )
  ON CONFLICT ON CONSTRAINT uq_sb_v15_project_batch_key DO UPDATE SET
    evidence_refs=(COALESCE(public.settlement_batches_v15.evidence_refs,'[]'::jsonb) || EXCLUDED.evidence_refs),
    run_id=EXCLUDED.run_id,
    trace_id=EXCLUDED.trace_id,
    policy_version_id=EXCLUDED.policy_version_id,
    updated_at=now();

  batch_id := v_id;
  status := 'open';
  RETURN NEXT;
END;
$$;

-- items_json: array of {provider_object_id,event_key?,posting_key?,currency,amount_minor,match_status}
CREATE OR REPLACE FUNCTION public.settlement_reconcile_dry_run_v15(
  p_project_id varchar,
  p_batch_key text,
  p_items jsonb,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(status text, matched_count int, unmatched_count int, ambiguous_count int)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_key text := btrim(coalesce(p_batch_key,''));
  v_run text := btrim(coalesce(p_run_id,''));
  v_trace text := btrim(coalesce(p_trace_id,''));
  v_policy text := btrim(coalesce(p_policy_version_id,''));
  v_batch_id uuid;
  e jsonb;
  v_mat int := 0;
  v_unm int := 0;
  v_amb int := 0;
  v_status text := 'reconciling';
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_key='' THEN RAISE EXCEPTION 'batch_key required' USING ERRCODE='22023'; END IF;
  IF v_run='' OR v_trace='' OR v_policy='' THEN RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023'; END IF;
  IF jsonb_typeof(p_items) <> 'array' THEN RAISE EXCEPTION 'items must be array' USING ERRCODE='22023'; END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023'; END IF;

  SELECT b.id INTO v_batch_id
  FROM public.settlement_batches_v15 b
  WHERE b.project_id=v_project_id::varchar(26) AND b.batch_key=v_key
  LIMIT 1;

  IF v_batch_id IS NULL THEN
    RAISE EXCEPTION 'batch not found' USING ERRCODE='23503';
  END IF;

  FOR e IN SELECT * FROM jsonb_array_elements(p_items)
  LOOP
    INSERT INTO public.settlement_items_v15(
      project_id, batch_id, provider_object_id, event_key, posting_key,
      currency, amount_minor, match_status, evidence_refs
    )
    VALUES(
      v_project_id::varchar(26), v_batch_id,
      (e->>'provider_object_id'),
      NULLIF(btrim(coalesce(e->>'event_key','')),'')::varchar(128),
      NULLIF(btrim(coalesce(e->>'posting_key','')),'')::char(64),
      (e->>'currency'),
      (e->>'amount_minor')::bigint,
      (e->>'match_status'),
      COALESCE(e->'evidence_refs','[]'::jsonb)
    )
    ON CONFLICT ON CONSTRAINT uq_si_v15_batch_provider_object DO NOTHING;
  END LOOP;

  SELECT count(*) FILTER (WHERE match_status='matched')::int,
         count(*) FILTER (WHERE match_status='unmatched')::int,
         count(*) FILTER (WHERE match_status='ambiguous')::int
    INTO v_mat, v_unm, v_amb
  FROM public.settlement_items_v15 si
  WHERE si.batch_id=v_batch_id;

  IF (v_unm + v_amb) > 0 THEN
    v_status := 'review_required';
  END IF;

  UPDATE public.settlement_batches_v15 b
     SET status=v_status,
         matched_count=v_mat,
         unmatched_count=v_unm,
         ambiguous_count=v_amb,
         run_id=v_run,
         trace_id=v_trace,
         policy_version_id=v_policy,
         evidence_refs=(COALESCE(b.evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
         updated_at=now()
   WHERE b.id=v_batch_id;

  status := v_status;
  matched_count := v_mat;
  unmatched_count := v_unm;
  ambiguous_count := v_amb;
  RETURN NEXT;
END;
$$;

-- =========================
-- SECURITY: revoke from PUBLIC
-- =========================
REVOKE ALL ON FUNCTION public.funds_op_open_v15(varchar,text,text,text,text,text,text,text,text,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.funds_op_resolve_v15(uuid,text,text,text,text,jsonb) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.refund_create_requested_v15(varchar,text,text,text,text,bigint,char(64),boolean,text,text,text,text,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.refund_mark_succeeded_from_utl_v15(varchar,text,varchar(128),text,text,text,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.refund_mark_failed_v15(varchar,text,text,text,boolean,text,text,text,jsonb) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.payout_schedule_v15(varchar,text,text,text,text,bigint,char(64),date,boolean,text,text,text,text,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.payout_mark_completed_from_utl_v15(varchar,text,varchar(128),text,text,text,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.payout_mark_failed_v15(varchar,text,text,text,boolean,timestamptz,text,text,text,jsonb) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.hold_create_v15(varchar,text,text,text,text,text,bigint,text,timestamptz,boolean,text,text,text,text,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.hold_release_v15(varchar,text,boolean,text,text,text,jsonb) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.dispute_upsert_from_utl_v15(varchar,text,text,text,bigint,text,timestamptz,timestamptz,varchar(128),text,text,text,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.dispute_event_insert_v15(varchar,text,varchar(128),text,timestamptz,uuid,text,text) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.settlement_batch_create_v15(varchar,text,text,timestamptz,timestamptz,text,boolean,text,text,text,text,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.settlement_reconcile_dry_run_v15(varchar,text,jsonb,text,text,text,jsonb) FROM PUBLIC;

COMMIT;