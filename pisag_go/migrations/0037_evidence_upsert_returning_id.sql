BEGIN;

CREATE OR REPLACE FUNCTION public.run_evidence_asset_upsert_id(
  p_run_id uuid,
  p_trace_id uuid,
  p_kind text,
  p_content_type text,
  p_byte_size integer,
  p_sha256 text,
  p_final_url text,
  p_stored_path text
)
RETURNS bigint
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_id bigint;
BEGIN
  IF p_run_id IS NULL THEN RAISE EXCEPTION 'run_id required' USING ERRCODE='22023'; END IF;
  IF p_trace_id IS NULL THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_kind::text,''))='' THEN RAISE EXCEPTION 'kind required' USING ERRCODE='22023'; END IF;
  IF p_byte_size IS NULL OR p_byte_size < 0 THEN RAISE EXCEPTION 'byte_size invalid' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_sha256::text,''))='' THEN RAISE EXCEPTION 'sha256 required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_final_url::text,''))='' THEN RAISE EXCEPTION 'final_url required' USING ERRCODE='22023'; END IF;
  IF btrim(coalesce(p_stored_path::text,''))='' THEN RAISE EXCEPTION 'stored_path required' USING ERRCODE='22023'; END IF;

  INSERT INTO public.run_evidence_assets(
    run_id, trace_id, kind, content_type, byte_size, sha256, final_url, stored_path
  )
  VALUES (
    p_run_id, p_trace_id, p_kind,
    NULLIF(btrim(coalesce(p_content_type::text,'')),''),
    p_byte_size, p_sha256, p_final_url, p_stored_path
  )
  ON CONFLICT (run_id, kind, sha256) DO UPDATE
    SET stored_path = EXCLUDED.stored_path
  RETURNING id INTO v_id;

  RETURN v_id;
END;
$$;

REVOKE ALL ON FUNCTION public.run_evidence_asset_upsert_id(
  uuid, uuid, text, text, integer, text, text, text
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.run_evidence_asset_upsert_id(
  uuid, uuid, text, text, integer, text, text, text
) TO ak_worker;

GRANT EXECUTE ON FUNCTION public.run_evidence_asset_upsert_id(
  uuid, uuid, text, text, integer, text, text, text
) TO ak;

COMMIT;