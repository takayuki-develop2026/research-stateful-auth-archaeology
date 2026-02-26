-- migrations/0036_claim_next_add_project_id.sql
BEGIN;

DROP FUNCTION IF EXISTS public.run_inputs_claim_next(text, text);

CREATE OR REPLACE FUNCTION public.run_inputs_claim_next(
  p_worker_id text,
  p_style     text DEFAULT 'cte_skip_locked'
)
RETURNS TABLE (
  id            bigint,
  project_id    varchar(26),
  run_id        uuid,
  trace_id      uuid,
  source_id     text,
  target_url    text,
  method        text,
  headers_json  jsonb,
  allowlist_key text,
  enqueue_key   text
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF p_worker_id IS NULL OR btrim(p_worker_id) = '' THEN
    RAISE EXCEPTION 'worker_id is required';
  END IF;

  RETURN QUERY
  WITH picked AS (
    SELECT ri2.id
    FROM public.run_inputs AS ri2
    WHERE ri2.claim_status='pending'
      AND ri2.next_attempt_at <= now()
    ORDER BY ri2.created_at ASC, ri2.id ASC
    FOR UPDATE OF ri2 SKIP LOCKED
    LIMIT 1
  )
  UPDATE public.run_inputs AS ri
  SET claim_status  = 'claimed',
      claimed_at    = now(),
      claimed_by    = p_worker_id,
      attempt_count = ri.attempt_count + 1
  FROM picked p, public.runs r
  WHERE ri.id = p.id
    AND r.run_id = ri.run_id
  RETURNING
    ri.id,
    r.project_id::varchar(26),
    ri.run_id,
    r.trace_id,
    ri.source_id,
    ri.target_url,
    ri.method,
    ri.headers_json,
    ri.allowlist_key,
    ri.enqueue_key;
END;
$$;

REVOKE ALL ON FUNCTION public.run_inputs_claim_next(text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.run_inputs_claim_next(text,text) TO ak_worker;

COMMIT;