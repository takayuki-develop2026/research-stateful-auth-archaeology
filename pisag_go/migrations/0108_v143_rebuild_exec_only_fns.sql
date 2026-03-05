-- 0108_v143_rebuild_exec_only_fns.sql
-- v14.3 exec-only functions:
-- - ledger_rebuild_run_accept_v14
-- - ledger_rebuild_run_mark_running_v14
-- - ledger_rebuild_run_mark_succeeded_v14
-- - ledger_rebuild_run_mark_failed_recorded_v14
-- - ledger_rebuild_run_dry_run_compute_v14 (core): compute diff summary from ledger_postings/entries

BEGIN;

-- helper: jsonb array check (reuse v14 helper if present)
-- we assume public._jsonb_is_array_v14 exists from v14.2
-- helper: sha256_hex_v14 exists too

CREATE OR REPLACE FUNCTION public.ledger_rebuild_run_accept_v14(
  p_project_id text,
  p_mode text, -- dry_run|apply
  p_from_ts timestamptz,
  p_to_ts timestamptz,
  p_idempotency_key text,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE(rebuild_run_id uuid, status text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id,''));
  v_mode text := btrim(coalesce(p_mode,''));
  v_run_id text := btrim(coalesce(p_run_id,''));
  v_trace_id text := btrim(coalesce(p_trace_id,''));
  v_policy text := btrim(coalesce(p_policy_version_id,''));
  v_idem text := btrim(coalesce(p_idempotency_key,''));
  v_existing uuid;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_mode NOT IN ('dry_run','apply') THEN RAISE EXCEPTION 'mode invalid' USING ERRCODE='22023'; END IF;
  IF v_idem='' THEN RAISE EXCEPTION 'idempotency_key required' USING ERRCODE='22023'; END IF;
  IF v_run_id='' THEN RAISE EXCEPTION 'run_id required' USING ERRCODE='22023'; END IF;
  IF v_trace_id='' THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;
  IF v_policy='' THEN RAISE EXCEPTION 'policy_version_id required' USING ERRCODE='22023'; END IF;
  IF NOT public._jsonb_is_array_v14(p_evidence_refs) THEN
    RAISE EXCEPTION 'evidence_refs must be a json array' USING ERRCODE='22023';
  END IF;

  PERFORM 1 FROM public.projects p WHERE p.id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  -- idempotency: use deterministic uuid from sha256(project_id|idem)
  SELECT (substring(public.sha256_hex_v14(v_project_id||'|'||v_idem),1,32))::uuid
    INTO v_existing;

  -- if already exists, return it
  IF EXISTS (SELECT 1 FROM public.ledger_rebuild_runs r WHERE r.id=v_existing) THEN
    rebuild_run_id := v_existing;
    status := 'accepted_exists';
    RETURN NEXT;
    RETURN;
  END IF;

  INSERT INTO public.ledger_rebuild_runs(
    id, project_id, mode, from_ts, to_ts,
    status, run_id, trace_id, policy_version_id,
    diff_summary, evidence_refs
  )
  VALUES(
    v_existing, v_project_id, v_mode, p_from_ts, p_to_ts,
    'accepted', v_run_id, v_trace_id, v_policy,
    '{}'::jsonb, p_evidence_refs
  );

  rebuild_run_id := v_existing;
  status := 'accepted_created';
  RETURN NEXT;
END;
$$;

CREATE OR REPLACE FUNCTION public.ledger_rebuild_run_mark_running_v14(
  p_rebuild_run_id uuid
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  UPDATE public.ledger_rebuild_runs
     SET status='running', updated_at=now()
   WHERE id=p_rebuild_run_id;
END;
$$;

CREATE OR REPLACE FUNCTION public.ledger_rebuild_run_mark_succeeded_v14(
  p_rebuild_run_id uuid,
  p_diff_summary jsonb DEFAULT '{}'::jsonb,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF jsonb_typeof(p_diff_summary) <> 'object' THEN
    RAISE EXCEPTION 'diff_summary must be an object' USING ERRCODE='22023';
  END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN
    RAISE EXCEPTION 'append_evidence_refs must be an array' USING ERRCODE='22023';
  END IF;

  UPDATE public.ledger_rebuild_runs
     SET status='succeeded',
         diff_summary=p_diff_summary,
         evidence_refs=(COALESCE(evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
         updated_at=now()
   WHERE id=p_rebuild_run_id;
END;
$$;

CREATE OR REPLACE FUNCTION public.ledger_rebuild_run_mark_failed_recorded_v14(
  p_rebuild_run_id uuid,
  p_diff_summary jsonb DEFAULT '{}'::jsonb,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF jsonb_typeof(p_diff_summary) <> 'object' THEN
    RAISE EXCEPTION 'diff_summary must be an object' USING ERRCODE='22023';
  END IF;
  IF NOT public._jsonb_is_array_v14(p_append_evidence_refs) THEN
    RAISE EXCEPTION 'append_evidence_refs must be an array' USING ERRCODE='22023';
  END IF;

  UPDATE public.ledger_rebuild_runs
     SET status='failed_recorded',
         diff_summary=p_diff_summary,
         evidence_refs=(COALESCE(evidence_refs,'[]'::jsonb) || p_append_evidence_refs),
         updated_at=now()
   WHERE id=p_rebuild_run_id;
END;
$$;

-- ============================================================
-- dry_run compute (P0): compute basic invariants from ledger_postings/entries
-- scope by postings.posted_at if present, else created_at fallback
-- Returns JSON summary (counts + sample keys)
-- ============================================================
CREATE OR REPLACE FUNCTION public.ledger_rebuild_run_dry_run_compute_v14(
  p_project_id text,
  p_from_ts timestamptz,
  p_to_ts timestamptz
)
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id,''));
  v_from timestamptz := p_from_ts;
  v_to timestamptz := p_to_ts;
  v_out jsonb;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_from IS NULL OR v_to IS NULL OR NOT (v_from < v_to) THEN
    RAISE EXCEPTION 'valid from/to required' USING ERRCODE='22023';
  END IF;

  -- out_of_balance postings: debit != credit
  WITH totals AS (
    SELECT
      p.posting_key,
      COALESCE(SUM(CASE WHEN e.direction='debit' THEN e.amount ELSE 0 END),0)::bigint AS debit_total,
      COALESCE(SUM(CASE WHEN e.direction='credit' THEN e.amount ELSE 0 END),0)::bigint AS credit_total
    FROM public.ledger_postings p
    JOIN public.ledger_entries e ON e.posting_id=p.id
    WHERE p.project_id=v_project_id
      AND p.posted_at >= v_from
      AND p.posted_at <  v_to
    GROUP BY p.posting_key
  ),
  oob AS (
    SELECT posting_key FROM totals WHERE debit_total <> credit_total
  ),
  currency_mismatch AS (
    SELECT p.posting_key
    FROM public.ledger_postings p
    JOIN public.ledger_entries e ON e.posting_id=p.id
    WHERE p.project_id=v_project_id
      AND p.posted_at >= v_from
      AND p.posted_at <  v_to
      AND e.currency <> p.currency
    GROUP BY p.posting_key
  )
  SELECT jsonb_build_object(
    'window', jsonb_build_object('from', v_from, 'to', v_to),
    'out_of_balance_count', (SELECT count(*) FROM oob),
    'currency_mismatch_count', (SELECT count(*) FROM currency_mismatch),
    'out_of_balance_sample', (SELECT COALESCE(jsonb_agg(posting_key) FILTER (WHERE posting_key IS NOT NULL), '[]'::jsonb) FROM (SELECT posting_key FROM oob LIMIT 10) s),
    'currency_mismatch_sample', (SELECT COALESCE(jsonb_agg(posting_key) FILTER (WHERE posting_key IS NOT NULL), '[]'::jsonb) FROM (SELECT posting_key FROM currency_mismatch LIMIT 10) s)
  ) INTO v_out;

  RETURN v_out;
END;
$$;

-- Fail-closed: revoke from PUBLIC
REVOKE ALL ON FUNCTION public.ledger_rebuild_run_accept_v14(text,text,timestamptz,timestamptz,text,text,text,text,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.ledger_rebuild_run_mark_running_v14(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.ledger_rebuild_run_mark_succeeded_v14(uuid,jsonb,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.ledger_rebuild_run_mark_failed_recorded_v14(uuid,jsonb,jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.ledger_rebuild_run_dry_run_compute_v14(text,timestamptz,timestamptz) FROM PUBLIC;

COMMIT;