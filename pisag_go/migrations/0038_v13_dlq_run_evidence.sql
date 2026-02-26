BEGIN;

-- 1) Expand: add nullable columns that reference run_evidence_assets
ALTER TABLE public.dlq_items_v13
  ADD COLUMN IF NOT EXISTS payload_run_evidence_asset_id bigint NULL REFERENCES public.run_evidence_assets(id) ON DELETE RESTRICT;

ALTER TABLE public.dlq_items_v13
  ADD COLUMN IF NOT EXISTS last_error_run_evidence_asset_id bigint NULL REFERENCES public.run_evidence_assets(id) ON DELETE RESTRICT;

-- 2) New EXECUTE ONLY function: enqueue with run_evidence_assets payload
CREATE OR REPLACE FUNCTION public.dlq_enqueue_run_evidence_v13(
  p_project_id varchar,
  p_run_id uuid,
  p_trace_id uuid,
  p_task_type varchar,
  p_source varchar,
  p_correlation_key varchar,
  p_payload_run_evidence_asset_id bigint,
  p_last_error_run_evidence_asset_id bigint
)
RETURNS bigint
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_source text := lower(btrim(coalesce(p_source::text,'')));
  v_task text := btrim(coalesce(p_task_type::text,''));
  v_id bigint;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF p_trace_id IS NULL THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;
  IF v_task='' THEN RAISE EXCEPTION 'task_type required' USING ERRCODE='22023'; END IF;
  IF v_source NOT IN ('queue','scheduler','webhook','manual') THEN RAISE EXCEPTION 'source invalid' USING ERRCODE='22023'; END IF;
  IF p_payload_run_evidence_asset_id IS NULL OR p_payload_run_evidence_asset_id<=0 THEN
    RAISE EXCEPTION 'payload_run_evidence_asset_id required' USING ERRCODE='22023';
  END IF;

  PERFORM 1 FROM public.projects WHERE id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  PERFORM 1 FROM public.run_evidence_assets WHERE id=p_payload_run_evidence_asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'payload run_evidence_asset not found' USING ERRCODE='23503'; END IF;

  IF p_last_error_run_evidence_asset_id IS NOT NULL THEN
    PERFORM 1 FROM public.run_evidence_assets WHERE id=p_last_error_run_evidence_asset_id;
    IF NOT FOUND THEN RAISE EXCEPTION 'error run_evidence_asset not found' USING ERRCODE='23503'; END IF;
  END IF;

  IF p_run_id IS NOT NULL THEN
    PERFORM 1 FROM public.runs WHERE run_id=p_run_id;
    IF NOT FOUND THEN RAISE EXCEPTION 'run not found: %', p_run_id USING ERRCODE='23503'; END IF;
  END IF;

  INSERT INTO public.dlq_items_v13(
    project_id, run_id, trace_id, task_type, source, correlation_key,
    payload_evidence_asset_id, last_error_evidence_asset_id,
    payload_run_evidence_asset_id, last_error_run_evidence_asset_id,
    attempts, status, created_at, updated_at
  )
  VALUES (
    v_project_id::varchar(26), p_run_id, p_trace_id,
    v_task::varchar(64), v_source::varchar(16),
    NULLIF(btrim(coalesce(p_correlation_key::text,'')),'')::varchar(128),

    NULL, NULL, -- old evidence_assets-based payload unused here
    p_payload_run_evidence_asset_id,
    p_last_error_run_evidence_asset_id,

    0, 'pending', now(), now()
  )
  RETURNING dlq_id INTO v_id;

  RETURN v_id;
END;
$$;

REVOKE ALL ON FUNCTION public.dlq_enqueue_run_evidence_v13(
  varchar, uuid, uuid, varchar, varchar, varchar, bigint, bigint
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.dlq_enqueue_run_evidence_v13(
  varchar, uuid, uuid, varchar, varchar, varchar, bigint, bigint
) TO ak;

GRANT EXECUTE ON FUNCTION public.dlq_enqueue_run_evidence_v13(
  varchar, uuid, uuid, varchar, varchar, varchar, bigint, bigint
) TO ak_worker;

COMMIT;