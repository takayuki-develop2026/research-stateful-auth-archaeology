BEGIN;

-- Retire a published version (turn into retired). Does NOT delete.
CREATE OR REPLACE FUNCTION gov_policy.policy_version_retire_v12(
  p_policy_set_id uuid,
  p_version_id uuid,
  p_trace_id text
) RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = gov_policy, pg_catalog
AS $$
DECLARE v_status text;
BEGIN
  SELECT status INTO v_status
    FROM gov_policy.policy_versions
   WHERE id = p_version_id AND policy_set_id = p_policy_set_id;

  IF v_status IS DISTINCT FROM 'published' THEN
    RETURN FALSE;
  END IF;

  UPDATE gov_policy.policy_versions
     SET status = 'retired'
   WHERE id = p_version_id AND policy_set_id = p_policy_set_id;

  -- If this was active, clear active (default deny)
  UPDATE gov_policy.policy_sets
     SET active_published_version_id = NULL,
         updated_at = now()
   WHERE id = p_policy_set_id
     AND active_published_version_id = p_version_id;

  RETURN TRUE;
EXCEPTION WHEN others THEN
  RETURN FALSE;
END
$$;

REVOKE ALL ON FUNCTION gov_policy.policy_version_retire_v12(uuid,uuid,text) FROM PUBLIC;

DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['ak_admin','ak_admin_api','ak_go_worker','ak_worker','ak_exec'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname=r) THEN
      EXECUTE format('GRANT EXECUTE ON FUNCTION gov_policy.policy_version_retire_v12(uuid,uuid,text) TO %I', r);
    END IF;
  END LOOP;
END
$$;

COMMIT;