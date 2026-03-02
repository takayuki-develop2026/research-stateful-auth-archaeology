-- 0092_v23_decision_status_read_fn.sql
-- v23: read decision status via SECURITY DEFINER (exec-only compatible)
BEGIN;

CREATE OR REPLACE FUNCTION public.decision_status_get_v23(
  p_project_id TEXT,
  p_decision_id BIGINT
)
RETURNS TABLE(status TEXT, decision_kind TEXT)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path TO public
AS $function$
BEGIN
  RETURN QUERY
    SELECT d.status, d.decision_kind
    FROM public.decision_ledgers_v23 d
    WHERE d.project_id = p_project_id
      AND d.id = p_decision_id
    LIMIT 1;
END;
$function$;

-- revoke public, grant to decisioncoresvc if exists
REVOKE ALL ON FUNCTION public.decision_status_get_v23(TEXT,BIGINT) FROM PUBLIC;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='decisioncoresvc') THEN
    GRANT EXECUTE ON FUNCTION public.decision_status_get_v23(TEXT,BIGINT) TO decisioncoresvc;
  END IF;
END $$;

COMMIT;