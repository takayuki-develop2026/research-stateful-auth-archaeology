BEGIN;

CREATE OR REPLACE FUNCTION gov_policy.policy_sets_list_v12(
  p_project_key text
) RETURNS TABLE (
  id uuid,
  project_key text,
  name varchar,
  description text,
  active_published_version_id uuid,
  status text,
  created_at timestamptz,
  updated_at timestamptz
)
LANGUAGE sql
SECURITY DEFINER
SET search_path = gov_policy, pg_catalog
AS $$
  SELECT
    s.id,
    s.project_key,
    s.name,
    s.description,
    s.active_published_version_id,
    s.status,
    s.created_at,
    s.updated_at
  FROM gov_policy.policy_sets s
  WHERE s.project_key = p_project_key
  ORDER BY s.created_at DESC
$$;

REVOKE ALL ON FUNCTION gov_policy.policy_sets_list_v12(text) FROM PUBLIC;

DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['ak_admin','ak_admin_api','ak_go_worker','ak_worker','ak_exec'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname=r) THEN
      EXECUTE format('GRANT EXECUTE ON FUNCTION gov_policy.policy_sets_list_v12(text) TO %I', r);
    END IF;
  END LOOP;
END
$$;

COMMIT;