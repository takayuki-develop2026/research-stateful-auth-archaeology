BEGIN;

CREATE OR REPLACE FUNCTION gov_policy.policy_set_create_v12b(
  p_project_key text,
  p_name text,
  p_description text,
  p_trace_id text
) RETURNS uuid
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = gov_policy, pg_catalog
AS $$
DECLARE v_id uuid;
BEGIN
  BEGIN
    INSERT INTO gov_policy.policy_sets(project_key, project_id, name, description, status)
    VALUES (p_project_key, NULL, p_name, p_description, 'active')
    RETURNING id INTO v_id;
  EXCEPTION WHEN unique_violation THEN
    SELECT id INTO v_id
      FROM gov_policy.policy_sets
     WHERE project_key = p_project_key AND name = p_name;
  END;

  RETURN v_id;
EXCEPTION WHEN others THEN
  RETURN NULL;
END
$$;

REVOKE ALL ON FUNCTION gov_policy.policy_set_create_v12b(text,text,text,text) FROM PUBLIC;

DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['ak_admin','ak_admin_api','ak_go_worker','ak_worker','ak_exec'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname=r) THEN
      EXECUTE format('GRANT EXECUTE ON FUNCTION gov_policy.policy_set_create_v12b(text,text,text,text) TO %I', r);
    END IF;
  END LOOP;
END
$$;

COMMIT;