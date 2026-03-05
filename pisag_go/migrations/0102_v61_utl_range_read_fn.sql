-- 0102_v61_utl_range_read_fn.sql
-- v6.1 UTL: range read function (EXECUTE ONLY)
-- Purpose:
-- - list UTL events in a time window for v14.1.2 range ingest
-- - deterministic ordering, bounded limit, optional status filter
-- - no direct table grants; use SECURITY DEFINER + REVOKE PUBLIC

BEGIN;

-- ============================================================
-- Range list (read-only)
-- Returns only the columns v14 needs for posting creation
-- ============================================================
CREATE OR REPLACE FUNCTION public.utl_list_events_range_v6(
  p_project_id varchar,
  p_from_ts timestamptz,
  p_to_ts timestamptz,
  p_status varchar DEFAULT NULL,     -- ingested|duplicate|processed|needs_retry|review_required or NULL=all
  p_limit int DEFAULT 500            -- bounded
)
RETURNS TABLE (
  id bigint,
  project_id varchar(26),
  event_key varchar(128),
  posting_key char(64),
  event_time timestamptz,
  received_at timestamptz,
  provider varchar(32),
  event_name varchar(64),
  provider_object_id varchar(128),
  trace_id uuid,
  run_id uuid,
  amount_minor bigint,
  currency char(3),
  status varchar(16),
  payload_evidence_asset_id bigint
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_status text := NULLIF(lower(btrim(coalesce(p_status::text,''))), '');
  v_limit int := COALESCE(p_limit, 500);
BEGIN
  IF v_project_id = '' THEN
    RAISE EXCEPTION 'project_id required' USING ERRCODE='22023';
  END IF;
  IF p_from_ts IS NULL OR p_to_ts IS NULL OR NOT (p_from_ts < p_to_ts) THEN
    RAISE EXCEPTION 'valid from_ts/to_ts required' USING ERRCODE='22023';
  END IF;

  -- hard bound (fail-closed)
  IF v_limit < 1 THEN v_limit := 1; END IF;
  IF v_limit > 5000 THEN v_limit := 5000; END IF;

  -- project existence (FIX: qualify id to avoid ambiguity with RETURNS TABLE column "id")
  PERFORM 1 FROM public.projects p WHERE p.id = v_project_id::varchar(26);
  IF NOT FOUND THEN
    RAISE EXCEPTION 'project not found' USING ERRCODE='23503';
  END IF;

  -- status validation (optional)
  IF v_status IS NOT NULL THEN
    IF v_status NOT IN ('ingested','duplicate','processed','needs_retry','review_required') THEN
      RAISE EXCEPTION 'status invalid' USING ERRCODE='22023';
    END IF;
  END IF;

  RETURN QUERY
  SELECT
    e.id,
    e.project_id,
    e.event_key,
    e.posting_key,
    e.event_time,
    e.received_at,
    e.provider,
    e.event_name,
    e.provider_object_id,
    e.trace_id,
    e.run_id,
    e.amount_minor,
    e.currency,
    e.status,
    e.payload_evidence_asset_id
  FROM public.universal_events_v6 e
  WHERE e.project_id = v_project_id::varchar(26)
    AND e.received_at >= p_from_ts
    AND e.received_at <  p_to_ts
    AND (v_status IS NULL OR e.status = v_status::varchar(16))
  ORDER BY e.received_at ASC, e.id ASC
  LIMIT v_limit;

END;
$$;

-- ============================================================
-- Permissions (EXECUTE ONLY)
-- ============================================================
REVOKE ALL ON FUNCTION public.utl_list_events_range_v6(
  varchar, timestamptz, timestamptz, varchar, int
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.utl_list_events_range_v6(
  varchar, timestamptz, timestamptz, varchar, int
) TO ak;

COMMIT;