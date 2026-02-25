-- migrations/0032_v20_ledger_gate.sql
-- v20 P0-2: enforce approval ledger gate at DB layer
-- Replace proposal_mark_approved_v20 to require approval_request_id (uuid) and verify:
--   approval_requests.status = 'approved' AND approval_requests.project_id = p_project_id

BEGIN;

-- 1) Drop old signature (safe even if missing)
DROP FUNCTION IF EXISTS public.proposal_mark_approved_v20(
  varchar, bigint, varchar, timestamptz
);

-- 2) Create new gated signature
CREATE OR REPLACE FUNCTION public.proposal_mark_approved_v20(
  p_project_id varchar,
  p_proposal_id bigint,
  p_approval_request_id uuid,
  p_approved_by_user_id varchar,
  p_approved_at_utc timestamptz
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_status text;
BEGIN
  IF v_project_id = '' THEN
    RAISE EXCEPTION 'project_id required' USING ERRCODE='22023';
  END IF;
  IF p_proposal_id IS NULL OR p_proposal_id <= 0 THEN
    RAISE EXCEPTION 'proposal_id required' USING ERRCODE='22023';
  END IF;
  IF p_approval_request_id IS NULL THEN
    RAISE EXCEPTION 'approval_request_id required' USING ERRCODE='22023';
  END IF;

  -- Guard: approval ledger must be approved and must match project
  SELECT ar.status INTO v_status
  FROM public.approval_requests ar
  WHERE ar.request_id = p_approval_request_id
    AND ar.project_id = v_project_id
  LIMIT 1;

  IF v_status IS NULL THEN
    RAISE EXCEPTION 'approval_request not found or project mismatch' USING ERRCODE='23503';
  END IF;

  IF v_status <> 'approved' THEN
    RAISE EXCEPTION 'approval_request is not approved' USING ERRCODE='22023';
  END IF;

  -- Mark proposal approved ONLY from needs_review state
  UPDATE public.remediation_proposals
  SET status='approved',
      approved_by_user_id=NULLIF(btrim(coalesce(p_approved_by_user_id::text,'')),'')::varchar(128),
      approved_at_utc=COALESCE(p_approved_at_utc, now()),
      updated_at=now()
  WHERE id = p_proposal_id
    AND project_id = v_project_id::varchar(26)
    AND status = 'needs_review';

  IF NOT FOUND THEN
    RAISE EXCEPTION 'proposal not found or not needs_review' USING ERRCODE='22023';
  END IF;
END;
$$;

-- 3) Permissions (EXECUTE ONLY)
REVOKE ALL ON FUNCTION public.proposal_mark_approved_v20(
  varchar, bigint, uuid, varchar, timestamptz
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.proposal_mark_approved_v20(
  varchar, bigint, uuid, varchar, timestamptz
) TO ak;

COMMIT;