-- 0123_v152_payout_hold_exec_only_fns.sql
-- v15.2 P0: payout_hold (create on schedule, consume on completed)
--
-- Adds:
-- - hold_consume_v152: active -> consumed
-- - payout_schedule_with_hold_v152: payout_schedule_v15 + hold_create_v15 + link payouts_v15.related_hold_key
-- - payout_mark_completed_consume_hold_from_utl_v152: payout_mark_completed_from_utl_v15 + consume linked hold
--
-- Notes:
-- - Uses existing exec-only functions from v15:
--   payout_schedule_v15, payout_mark_completed_from_utl_v15, hold_create_v15
-- - Uses v14 helpers: public._jsonb_is_array_v14
-- - Fail-closed: requires confirm=true, run_id/trace_id/policy_version_id
-- - throw禁止運用に合わせて、ここではDB例外は投げる（入力不正/存在しない等は契約違反）
--   失敗を review_required に落とすのは「サービス層」でやるのがv14/v15のあなたの流儀（後でfundssvc化する時も同じ）。

BEGIN;

-- =========================================================
-- 1) Hold consume (active -> consumed)
-- =========================================================
CREATE OR REPLACE FUNCTION public.hold_consume_v152(
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
  IF btrim(coalesce(p_project_id::text,''))='' THEN
    RAISE EXCEPTION 'project_id required' USING ERRCODE='22023';
  END IF;
  IF btrim(coalesce(p_hold_key,''))='' THEN
    RAISE EXCEPTION 'hold_key required' USING ERRCODE='22023';
  END IF;
  IF p_confirm IS DISTINCT FROM true THEN
    RAISE EXCEPTION 'confirm=true required' USING ERRCODE='22023';
  END IF;
  IF btrim(coalesce(p_run_id,''))='' OR btrim(coalesce(p_trace_id,''))='' OR btrim(coalesce(p_policy_version_id,''))='' THEN
    RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023';
  END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN
    RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023';
  END IF;

  UPDATE public.holds_v15 h
     SET status='consumed',
         evidence_refs=(COALESCE(h.evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
         run_id=p_run_id,
         trace_id=p_trace_id,
         policy_version_id=p_policy_version_id,
         updated_at=now()
   WHERE h.project_id=p_project_id::varchar(26)
     AND h.hold_key=p_hold_key
     AND h.status='active';
END;
$$;

-- =========================================================
-- 2) Payout schedule + hold create + link
-- =========================================================
CREATE OR REPLACE FUNCTION public.payout_schedule_with_hold_v152(
  p_project_id varchar,
  p_payout_key text,               -- optional; if empty -> internal:<uuid> (same as v15)
  p_provider text,
  p_shop_id text,
  p_currency text,
  p_amount_net_minor bigint,
  p_posting_key char(64),
  p_scheduled_for date,

  p_hold_key text,                 -- optional; if empty -> internal:<uuid> by hold_create_v15
  p_hold_expires_at timestamptz,   -- optional; can be NULL
  p_hold_reason text,              -- optional; if empty -> "payout_hold for <payout_key>"

  p_confirm boolean,
  p_idempotency_key text,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(payout_id uuid, payout_key text, payout_status text, hold_id uuid, hold_key text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_payout record;
  v_hold record;
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_reason text := btrim(coalesce(p_hold_reason,''));
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF p_confirm IS DISTINCT FROM true THEN RAISE EXCEPTION 'confirm=true required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_run_id,''))='' OR btrim(coalesce(p_trace_id,''))='' OR btrim(coalesce(p_policy_version_id,''))='' THEN
    RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023';
  END IF;
  IF NOT public._jsonb_is_array_v14(p_evidence_refs) THEN
    RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023';
  END IF;

  -- 2.1 schedule payout (v15)
  SELECT * INTO v_payout
  FROM public.payout_schedule_v15(
    p_project_id,
    p_payout_key,
    p_provider,
    p_shop_id,
    p_currency,
    p_amount_net_minor,
    p_posting_key,
    p_scheduled_for,
    p_confirm,
    p_idempotency_key,
    p_run_id,
    p_trace_id,
    p_policy_version_id,
    p_evidence_refs
  );

  IF v_reason='' THEN
    v_reason := 'payout_hold for ' || v_payout.payout_key;
  END IF;

  -- 2.2 create hold (payout_hold) (v15)
  SELECT * INTO v_hold
  FROM public.hold_create_v15(
    p_project_id,
    p_hold_key,
    'payout_hold',
    'shop',
    p_shop_id,
    p_currency,
    p_amount_net_minor,
    v_reason,
    p_hold_expires_at,
    p_confirm,
    p_idempotency_key || ':hold',  -- scoped derivation (still deterministic/idempotent)
    p_run_id,
    p_trace_id,
    p_policy_version_id,
    p_evidence_refs
  );

  -- 2.3 link payout -> hold_key
  UPDATE public.payouts_v15 p
     SET related_hold_key = v_hold.hold_key,
         evidence_refs = (COALESCE(p.evidence_refs,'[]'::jsonb) || p_evidence_refs),
         run_id = p_run_id,
         trace_id = p_trace_id,
         policy_version_id = p_policy_version_id,
         updated_at = now()
   WHERE p.project_id = p_project_id::varchar(26)
     AND p.payout_key = v_payout.payout_key;

  payout_id := v_payout.payout_id;
  payout_key := v_payout.payout_key;
  payout_status := v_payout.status;
  hold_id := v_hold.hold_id;
  hold_key := v_hold.hold_key;
  RETURN NEXT;
END;
$$;

-- =========================================================
-- 3) mark completed + consume hold (UTL mock or real webhook)
-- =========================================================
CREATE OR REPLACE FUNCTION public.payout_mark_completed_consume_hold_from_utl_v152(
  p_project_id varchar,
  p_payout_key text,
  p_source_event_key varchar(128),
  p_confirm boolean,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(payout_key text, payout_status text, hold_key text, hold_status text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_key text := btrim(coalesce(p_payout_key,''));
  v_hold_key text;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_key='' THEN RAISE EXCEPTION 'payout_key required' USING ERRCODE='22023'; END IF;
  IF p_confirm IS DISTINCT FROM true THEN RAISE EXCEPTION 'confirm=true required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_run_id,''))='' OR btrim(coalesce(p_trace_id,''))='' OR btrim(coalesce(p_policy_version_id,''))='' THEN
    RAISE EXCEPTION 'run_id/trace_id/policy_version_id required' USING ERRCODE='22023';
  END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN
    RAISE EXCEPTION 'evidence_refs must be array' USING ERRCODE='22023';
  END IF;

  -- 3.1 mark completed (v15)
  PERFORM public.payout_mark_completed_from_utl_v15(
    p_project_id,
    p_payout_key,
    p_source_event_key,
    p_run_id,
    p_trace_id,
    p_policy_version_id,
    p_append_evidence_refs
  );

  -- 3.2 read related hold_key
  SELECT p.related_hold_key INTO v_hold_key
  FROM public.payouts_v15 p
  WHERE p.project_id = p_project_id::varchar(26)
    AND p.payout_key = p_payout_key
  LIMIT 1;

  -- 3.3 consume if exists
  IF v_hold_key IS NOT NULL AND btrim(v_hold_key)<>'' THEN
    PERFORM public.hold_consume_v152(
      p_project_id,
      v_hold_key,
      true,
      p_run_id,
      p_trace_id,
      p_policy_version_id,
      p_append_evidence_refs
    );

    payout_key := p_payout_key;
    payout_status := 'completed';
    hold_key := v_hold_key;
    hold_status := 'consumed';
    RETURN NEXT;
    RETURN;
  END IF;

  payout_key := p_payout_key;
  payout_status := 'completed';
  hold_key := NULL;
  hold_status := 'no_hold';
  RETURN NEXT;
END;
$$;

-- =========================================================
-- SECURITY: revoke from PUBLIC
-- =========================================================
REVOKE ALL ON FUNCTION public.hold_consume_v152(varchar,text,boolean,text,text,text,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.payout_schedule_with_hold_v152(varchar,text,text,text,text,bigint,char(64),date,text,timestamptz,text,boolean,text,text,text,text,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.payout_mark_completed_consume_hold_from_utl_v152(varchar,text,varchar(128),boolean,text,text,text,jsonb) FROM PUBLIC;

COMMIT;