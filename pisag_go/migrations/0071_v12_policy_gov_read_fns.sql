BEGIN;

-- active version for a set (project_key guard)
CREATE OR REPLACE FUNCTION gov_policy.policy_set_active_v12(
  p_project_key text,
  p_policy_set_id uuid
) RETURNS TABLE (
  policy_set_id uuid,
  active_published_version_id uuid
)
LANGUAGE sql
SECURITY DEFINER
SET search_path = gov_policy, pg_catalog
AS $$
  SELECT s.id, s.active_published_version_id
    FROM gov_policy.policy_sets s
   WHERE s.project_key = p_project_key
     AND s.id = p_policy_set_id
$$;

-- version detail (project_key guard via join)
CREATE OR REPLACE FUNCTION gov_policy.policy_version_get_v12(
  p_project_key text,
  p_version_id uuid
) RETURNS TABLE (
  id uuid,
  policy_set_id uuid,
  version_number int,
  status text,
  compiled_policy_evidence_asset_id uuid,
  compiled_policy_checksum char(64),
  published_by text,
  published_at timestamptz,
  publish_reason text,
  previous_version_id uuid
)
LANGUAGE sql
SECURITY DEFINER
SET search_path = gov_policy, pg_catalog
AS $$
  SELECT
    v.id, v.policy_set_id, v.version_number, v.status,
    v.compiled_policy_evidence_asset_id, v.compiled_policy_checksum,
    v.published_by, v.published_at, v.publish_reason, v.previous_version_id
  FROM gov_policy.policy_versions v
  JOIN gov_policy.policy_sets s ON s.id = v.policy_set_id
  WHERE s.project_key = p_project_key
    AND v.id = p_version_id
$$;

REVOKE ALL ON FUNCTION gov_policy.policy_set_active_v12(text,uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION gov_policy.policy_version_get_v12(text,uuid) FROM PUBLIC;

DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['ak_admin','ak_admin_api','ak_go_worker','ak_worker','ak_exec'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname=r) THEN
      EXECUTE format('GRANT EXECUTE ON FUNCTION gov_policy.policy_set_active_v12(text,uuid) TO %I', r);
      EXECUTE format('GRANT EXECUTE ON FUNCTION gov_policy.policy_version_get_v12(text,uuid) TO %I', r);
    END IF;
  END LOOP;
END
$$;

COMMIT;