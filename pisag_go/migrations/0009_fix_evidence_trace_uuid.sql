BEGIN;

DO $$
DECLARE
  bad_count bigint;
BEGIN
  SELECT count(*)
    INTO bad_count
  FROM public.run_evidence_assets
  WHERE trace_id IS NOT NULL
    AND trace_id::text !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$';

  IF bad_count > 0 THEN
    RAISE EXCEPTION 'run_evidence_assets.trace_id has % non-uuid rows', bad_count;
  END IF;
END
$$;

COMMIT;
