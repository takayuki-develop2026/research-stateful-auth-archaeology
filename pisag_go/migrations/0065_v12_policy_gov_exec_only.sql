BEGIN;

REVOKE ALL ON ALL TABLES IN SCHEMA gov_policy FROM PUBLIC;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA gov_policy FROM PUBLIC;

-- ------------------------------------------------------------
-- create policy_set (idempotent by project_id+name)
-- ------------------------------------------------------------
CREATE OR REPLACE FUNCTION gov_policy.policy_set_create_v12(
  p_project_id uuid,
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
    INSERT INTO gov_policy.policy_sets(project_id,name,description,status)
    VALUES (p_project_id, p_name, p_description, 'active')
    RETURNING id INTO v_id;
  EXCEPTION WHEN unique_violation THEN
    SELECT id INTO v_id
      FROM gov_policy.policy_sets
     WHERE project_id=p_project_id AND name=p_name;
  END;
  RETURN v_id;
EXCEPTION WHEN others THEN
  RETURN NULL;
END
$$;

-- ------------------------------------------------------------
-- publish version (creates published policy_version)
-- also enforces active_published_version_id points to published only
-- ------------------------------------------------------------
CREATE OR REPLACE FUNCTION gov_policy.policy_version_publish_v12(
  p_policy_set_id uuid,
  p_compiled_policy_evidence_asset_id uuid,
  p_compiled_policy_checksum char(64),
  p_published_by text,
  p_publish_reason text,
  p_trace_id text
) RETURNS uuid
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = gov_policy, pg_catalog
AS $$
DECLARE
  v_prev uuid;
  v_next int;
  v_new uuid;
BEGIN
  SELECT active_published_version_id INTO v_prev
    FROM gov_policy.policy_sets
   WHERE id = p_policy_set_id;

  SELECT COALESCE(MAX(version_number), 0) + 1 INTO v_next
    FROM gov_policy.policy_versions
   WHERE policy_set_id = p_policy_set_id;

  INSERT INTO gov_policy.policy_versions(
    policy_set_id, version_number, status,
    compiled_policy_evidence_asset_id, compiled_policy_checksum,
    published_by, published_at, publish_reason,
    previous_version_id
  )
  VALUES(
    p_policy_set_id, v_next, 'published',
    p_compiled_policy_evidence_asset_id, p_compiled_policy_checksum,
    p_published_by, now(), p_publish_reason,
    v_prev
  )
  RETURNING id INTO v_new;

  -- set active published version
  UPDATE gov_policy.policy_sets
     SET active_published_version_id = v_new,
         updated_at = now()
   WHERE id = p_policy_set_id;

  RETURN v_new;
EXCEPTION WHEN others THEN
  RETURN NULL;
END
$$;

-- ------------------------------------------------------------
-- rollback active to an existing published version
-- ------------------------------------------------------------
CREATE OR REPLACE FUNCTION gov_policy.policy_set_rollback_v12(
  p_policy_set_id uuid,
  p_to_version_id uuid,
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
   WHERE id = p_to_version_id AND policy_set_id = p_policy_set_id;

  IF v_status IS DISTINCT FROM 'published' THEN
    RETURN FALSE;
  END IF;

  UPDATE gov_policy.policy_sets
     SET active_published_version_id = p_to_version_id,
         updated_at = now()
   WHERE id = p_policy_set_id;

  RETURN TRUE;
EXCEPTION WHEN others THEN
  RETURN FALSE;
END
$$;

-- ------------------------------------------------------------
-- record publication (idempotent)
-- ------------------------------------------------------------
CREATE OR REPLACE FUNCTION gov_policy.policy_publication_record_v12(
  p_project_id uuid,
  p_policy_set_id uuid,
  p_action text,
  p_from_version_id uuid,
  p_to_version_id uuid,
  p_triggered_by text,
  p_reason text,
  p_incident_id text,
  p_status text,
  p_result_evidence_asset_id uuid,
  p_trace_id text,
  p_idempotency_key text
) RETURNS uuid
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = gov_policy, pg_catalog
AS $$
DECLARE v_id uuid;
BEGIN
  BEGIN
    INSERT INTO gov_policy.policy_publications(
      project_id, policy_set_id, action,
      from_version_id, to_version_id,
      triggered_by, reason, incident_id,
      status, result_evidence_asset_id,
      trace_id, idempotency_key
    )
    VALUES(
      p_project_id, p_policy_set_id, p_action,
      p_from_version_id, p_to_version_id,
      p_triggered_by, p_reason, p_incident_id,
      p_status, p_result_evidence_asset_id,
      p_trace_id, p_idempotency_key
    )
    RETURNING id INTO v_id;
  EXCEPTION WHEN unique_violation THEN
    SELECT id INTO v_id
      FROM gov_policy.policy_publications
     WHERE project_id=p_project_id AND idempotency_key=p_idempotency_key;
  END;
  RETURN v_id;
EXCEPTION WHEN others THEN
  RETURN NULL;
END
$$;

-- EXECUTE ONLY
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA gov_policy FROM PUBLIC;

DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['ak_admin','ak_admin_api','ak_go_worker','ak_worker','ak_exec'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname=r) THEN
      EXECUTE format('GRANT USAGE ON SCHEMA gov_policy TO %I', r);
      EXECUTE format('GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA gov_policy TO %I', r);
    END IF;
  END LOOP;
END
$$;

COMMIT;