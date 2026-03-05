-- 0103_v62_utl_status_exec_only.sql
-- v6.2 UTL: status update exec-only functions for orchestration (ledger ingest)
-- Goals:
-- - mark processed idempotently
-- - mark needs_retry with attempts++ and last_error_code/evidence
-- - never delete, never rewrite finance facts; only status + metadata

BEGIN;

-- ============================================================
-- 1) Mark processed (idempotent)
-- ============================================================
CREATE OR REPLACE FUNCTION public.utl_mark_processed_v6(
  p_project_id varchar,
  p_event_key varchar,
  p_trace_id uuid,
  p_run_id uuid DEFAULT NULL
)
RETURNS TABLE(status varchar(16))
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_event_key text := btrim(coalesce(p_event_key::text,''));
  v_cur_status text;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_event_key='' THEN RAISE EXCEPTION 'event_key required' USING ERRCODE='22023'; END IF;
  IF p_trace_id IS NULL THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects p WHERE p.id = v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  -- Lock the row to serialize status transitions
  SELECT e.status
    INTO v_cur_status
  FROM public.universal_events_v6 e
  WHERE e.project_id = v_project_id::varchar(26)
    AND e.event_key = v_event_key::varchar(128)
  FOR UPDATE;

  IF v_cur_status IS NULL THEN
    status := 'review_required';
    RETURN NEXT;
    RETURN;
  END IF;

  -- Do not touch duplicates
  IF v_cur_status = 'duplicate' THEN
    status := 'duplicate';
    RETURN NEXT;
    RETURN;
  END IF;

  -- Idempotent: already processed => keep
  IF v_cur_status = 'processed' THEN
    status := 'processed';
    RETURN NEXT;
    RETURN;
  END IF;

  UPDATE public.universal_events_v6
     SET status = 'processed',
         trace_id = p_trace_id,             -- latest trace for audit
         run_id = COALESCE(p_run_id, run_id),
         last_error_code = NULL,
         last_error_evidence_asset_id = NULL,
         updated_at = now()
   WHERE project_id = v_project_id::varchar(26)
     AND event_key = v_event_key::varchar(128);

  status := 'processed';
  RETURN NEXT;
END;
$$;

-- ============================================================
-- 2) Mark needs_retry (idempotent-ish)
-- - increments process_attempts
-- - sets last_error_code / last_error_evidence_asset_id
-- - status becomes needs_retry (unless already duplicate/processed)
-- ============================================================
CREATE OR REPLACE FUNCTION public.utl_mark_needs_retry_v6(
  p_project_id varchar,
  p_event_key varchar,
  p_trace_id uuid,
  p_run_id uuid DEFAULT NULL,
  p_error_code varchar DEFAULT NULL,
  p_error_evidence_asset_id bigint DEFAULT NULL
)
RETURNS TABLE(status varchar(16))
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_event_key text := btrim(coalesce(p_event_key::text,''));
  v_err text := NULLIF(btrim(coalesce(p_error_code::text,'')), '');
  v_cur_status text;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_event_key='' THEN RAISE EXCEPTION 'event_key required' USING ERRCODE='22023'; END IF;
  IF p_trace_id IS NULL THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects p WHERE p.id = v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  -- Validate evidence asset if provided (do not throw into 500; if missing, null it)
  IF p_error_evidence_asset_id IS NOT NULL THEN
    PERFORM 1 FROM public.evidence_assets ea
     WHERE ea.id = p_error_evidence_asset_id
       AND ea.project_id = v_project_id::varchar(26);
    IF NOT FOUND THEN
      p_error_evidence_asset_id := NULL;
    END IF;
  END IF;

  SELECT e.status
    INTO v_cur_status
  FROM public.universal_events_v6 e
  WHERE e.project_id = v_project_id::varchar(26)
    AND e.event_key = v_event_key::varchar(128)
  FOR UPDATE;

  IF v_cur_status IS NULL THEN
    status := 'review_required';
    RETURN NEXT;
    RETURN;
  END IF;

  IF v_cur_status IN ('duplicate','processed') THEN
    status := v_cur_status::varchar(16);
    RETURN NEXT;
    RETURN;
  END IF;

  UPDATE public.universal_events_v6
     SET status = 'needs_retry',
         process_attempts = process_attempts + 1,
         trace_id = p_trace_id,
         run_id = COALESCE(p_run_id, run_id),
         last_error_code = v_err,
         last_error_evidence_asset_id = p_error_evidence_asset_id,
         updated_at = now()
   WHERE project_id = v_project_id::varchar(26)
     AND event_key = v_event_key::varchar(128);

  status := 'needs_retry';
  RETURN NEXT;
END;
$$;

-- ============================================================
-- 3) Permissions (EXECUTE ONLY)
-- ============================================================
REVOKE ALL ON FUNCTION public.utl_mark_processed_v6(varchar,varchar,uuid,uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.utl_mark_needs_retry_v6(varchar,varchar,uuid,uuid,varchar,bigint) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.utl_mark_processed_v6(varchar,varchar,uuid,uuid) TO ak;
GRANT EXECUTE ON FUNCTION public.utl_mark_needs_retry_v6(varchar,varchar,uuid,uuid,varchar,bigint) TO ak;

COMMIT;