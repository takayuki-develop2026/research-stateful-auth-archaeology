-- migrations/0036_v13_worker_support.sql
-- v13 P2 support:
-- 1) run_evidence_assets insert must return evidence_assets.id (bigint) without worker SELECT
--    -> SECURITY DEFINER function returns bigint id (uses INSERT ... ON CONFLICT DO UPDATE ... RETURNING)
-- 2) run_inputs_claim_next must return project_id to allow DLQ enqueue (project_id required by dlq_items_v13)

BEGIN;

-- ------------------------------------------------------------
-- 1) Evidence: insert/upsert returning bigint id
-- ------------------------------------------------------------
CREATE OR REPLACE FUNCTION public.run_evidence_asset_upsert_id(
  p_run_id uuid,
  p_trace_id uuid,
  p_kind varchar,
  p_content_type varchar,
  p_byte_size bigint,
  p_sha256 varchar,
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

  -- This assumes run_evidence_assets has a bigint PK column named "id".
  -- If your table uses a different pk name, tell me and I'll adjust.
  INSERT INTO public.run_evidence_assets(
    run_id, trace_id, kind, content_type, byte_size, sha256, final_url, stored_path
  )
  VALUES (
    p_run_id,
    p_trace_id,
    p_kind,
    NULLIF(btrim(coalesce(p_content_type::text,'')),''),
    p_byte_size,
    p_sha256,
    NULLIF(btrim(coalesce(p_final_url::text,'')),''),
    NULLIF(btrim(coalesce(p_stored_path::text,'')),'')
  )
  ON CONFLICT (run_id, kind, sha256) DO UPDATE
    SET stored_path = EXCLUDED.stored_path
  RETURNING id INTO v_id;

  RETURN v_id;
END;
$$;

REVOKE ALL ON FUNCTION public.run_evidence_asset_upsert_id(
  uuid, uuid, varchar, varchar, bigint, varchar, text, text
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.run_evidence_asset_upsert_id(
  uuid, uuid, varchar, varchar, bigint, varchar, text, text
) TO ak_worker;

GRANT EXECUTE ON FUNCTION public.run_evidence_asset_upsert_id(
  uuid, uuid, varchar, varchar, bigint, varchar, text, text
) TO ak;

-- ------------------------------------------------------------
-- 2) ClaimNext: add project_id in return
-- ------------------------------------------------------------
-- IMPORTANT:
-- We assume your existing function name is public.run_inputs_claim_next(worker_id, style)
-- and it returns: id, run_id, trace_id, source_id, target_url, method, headers_json, allowlist_key, enqueue_key
-- We'll replace it to also return project_id as 2nd column (after id).
--
-- If your existing function body differs, paste migrations/0008_claim_next_fn.sql and I'll align it exactly.

CREATE OR REPLACE FUNCTION public.run_inputs_claim_next(
  p_worker_id text,
  p_style text
)
RETURNS TABLE (
  id bigint,
  project_id varchar(26),
  run_id uuid,
  trace_id uuid,
  source_id text,
  target_url text,
  method text,
  headers_json jsonb,
  allowlist_key text,
  enqueue_key text
)
LANGUAGE sql
SECURITY DEFINER
SET search_path = public
AS $$
  -- Minimal wrapper: call existing claim logic and join runs for project_id.
  -- If your original function already performs claim/lock, integrate it there.
  WITH claimed AS (
    SELECT *
    FROM public.run_inputs_claim_next__internal(p_worker_id, p_style)
  )
  SELECT
    c.id::bigint,
    r.project_id::varchar(26),
    c.run_id,
    c.trace_id,
    c.source_id,
    c.target_url,
    c.method,
    c.headers_json,
    c.allowlist_key,
    c.enqueue_key
  FROM claimed c
  JOIN public.runs r ON r.run_id = c.run_id;
$$;

-- Execute rights stay as they were (typically ak_worker)
REVOKE ALL ON FUNCTION public.run_inputs_claim_next(text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.run_inputs_claim_next(text, text) TO ak_worker;
GRANT EXECUTE ON FUNCTION public.run_inputs_claim_next(text, text) TO ak;

COMMIT;