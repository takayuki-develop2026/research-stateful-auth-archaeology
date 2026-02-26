-- migrations/0033_v20_slo_exec_only.sql
-- v20 P0-3: EXECUTE ONLY function for slo_evaluations upsert

BEGIN;

CREATE OR REPLACE FUNCTION public.slo_evaluation_upsert_v20(
  p_project_id varchar,
  p_slo_id bigint,
  p_evaluation_key text,
  p_window_start_at_utc timestamptz,
  p_window_end_at_utc timestamptz,
  p_sli_value numeric,
  p_error_budget_remaining numeric,
  p_status varchar,
  p_evaluation_evidence_asset_id bigint
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_key text := btrim(coalesce(p_evaluation_key::text,''));
  v_status text := lower(btrim(coalesce(p_status::text,'')));
BEGIN
  IF v_project_id = '' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF p_slo_id IS NULL OR p_slo_id <= 0 THEN RAISE EXCEPTION 'slo_id required' USING ERRCODE='22023'; END IF;
  IF v_key = '' THEN RAISE EXCEPTION 'evaluation_key required' USING ERRCODE='22023'; END IF;
  IF v_status NOT IN ('ok','warn','breach') THEN RAISE EXCEPTION 'status must be ok|warn|breach' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects WHERE id = v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found: %', v_project_id USING ERRCODE='23503'; END IF;

  PERFORM 1 FROM public.slo_definitions WHERE id = p_slo_id AND project_id = v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'slo_definition not found' USING ERRCODE='23503'; END IF;

  PERFORM 1 FROM public.evidence_assets WHERE id = p_evaluation_evidence_asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'evidence_asset not found: %', p_evaluation_evidence_asset_id USING ERRCODE='23503'; END IF;

  INSERT INTO public.slo_evaluations(
    project_id, slo_id, evaluation_key,
    window_start_at_utc, window_end_at_utc,
    sli_value, error_budget_remaining, status,
    evaluated_at_utc, evaluation_evidence_asset_id
  )
  VALUES (
    v_project_id::varchar(26),
    p_slo_id,
    v_key,
    p_window_start_at_utc,
    p_window_end_at_utc,
    p_sli_value,
    p_error_budget_remaining,
    v_status::varchar(16),
    now(),
    p_evaluation_evidence_asset_id
  )
  ON CONFLICT (project_id, evaluation_key) DO UPDATE
  SET
    sli_value = EXCLUDED.sli_value,
    error_budget_remaining = EXCLUDED.error_budget_remaining,
    status = EXCLUDED.status,
    evaluated_at_utc = now(),
    evaluation_evidence_asset_id = EXCLUDED.evaluation_evidence_asset_id;
END;
$$;

REVOKE ALL ON FUNCTION public.slo_evaluation_upsert_v20(
  varchar,bigint,text,timestamptz,timestamptz,numeric,numeric,varchar,bigint
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.slo_evaluation_upsert_v20(
  varchar,bigint,text,timestamptz,timestamptz,numeric,numeric,varchar,bigint
) TO ak;

COMMIT;