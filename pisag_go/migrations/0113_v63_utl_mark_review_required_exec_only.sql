-- 0113_v63_utl_mark_review_required_exec_only.sql
-- v6.3: exec-only status transition to review_required for close gating
-- Purpose:
-- - allow period close by excluding known-bad UTL events from "ingested/needs_retry" pipeline
-- - keep finance fact; only status changes (no delete)

BEGIN;

CREATE OR REPLACE FUNCTION public.utl_mark_review_required_v63(
  p_project_id varchar,
  p_event_key varchar,
  p_trace_id uuid,
  p_run_id uuid DEFAULT NULL,
  p_reason_code varchar DEFAULT NULL,
  p_reason_evidence_asset_id bigint DEFAULT NULL
)
RETURNS TABLE(status varchar(16))
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_event_key text := btrim(coalesce(p_event_key::text,''));
  v_reason text := NULLIF(btrim(coalesce(p_reason_code::text,'')), '');
  v_cur_status text;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_event_key='' THEN RAISE EXCEPTION 'event_key required' USING ERRCODE='22023'; END IF;
  IF p_trace_id IS NULL THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects p WHERE p.id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  -- validate evidence id if provided (best-effort: if missing, null it)
  IF p_reason_evidence_asset_id IS NOT NULL THEN
    PERFORM 1 FROM public.evidence_assets ea
     WHERE ea.id = p_reason_evidence_asset_id
       AND ea.project_id = v_project_id::varchar(26);
    IF NOT FOUND THEN
      p_reason_evidence_asset_id := NULL;
    END IF;
  END IF;

  SELECT e.status
    INTO v_cur_status
  FROM public.universal_events_v6 e
  WHERE e.project_id=v_project_id::varchar(26) AND e.event_key=v_event_key::varchar(128)
  FOR UPDATE;

  IF v_cur_status IS NULL THEN
    status := 'review_required';
    RETURN NEXT;
    RETURN;
  END IF;

  -- do not touch duplicates/processed (finance fact history)
  IF v_cur_status IN ('duplicate','processed') THEN
    status := v_cur_status::varchar(16);
    RETURN NEXT;
    RETURN;
  END IF;

  UPDATE public.universal_events_v6
     SET status = 'review_required',
         trace_id = p_trace_id,
         run_id = COALESCE(p_run_id, run_id),
         last_error_code = COALESCE(v_reason, last_error_code),
         last_error_evidence_asset_id = COALESCE(p_reason_evidence_asset_id, last_error_evidence_asset_id),
         updated_at = now()
   WHERE project_id=v_project_id::varchar(26) AND event_key=v_event_key::varchar(128);

  status := 'review_required';
  RETURN NEXT;
END;
$$;

REVOKE ALL ON FUNCTION public.utl_mark_review_required_v63(varchar,varchar,uuid,uuid,varchar,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.utl_mark_review_required_v63(varchar,varchar,uuid,uuid,varchar,bigint) TO ak;

COMMIT;