BEGIN;

CREATE OR REPLACE FUNCTION gov_policy.policy_publication_record_v12b(
  p_project_key text,
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
      project_key, project_id,
      policy_set_id, action,
      from_version_id, to_version_id,
      triggered_by, reason, incident_id,
      status, result_evidence_asset_id,
      trace_id, idempotency_key
    )
    VALUES(
      p_project_key, NULL,
      p_policy_set_id, p_action,
      p_from_version_id, p_to_version_id,
      p_triggered_by, p_reason, p_incident_id,
      p_status, p_result_evidence_asset_id,
      p_trace_id, p_idempotency_key
    )
    RETURNING id INTO v_id;
  EXCEPTION WHEN unique_violation THEN
    SELECT id INTO v_id
      FROM gov_policy.policy_publications
     WHERE project_key=p_project_key AND idempotency_key=p_idempotency_key;
  END;

  RETURN v_id;
EXCEPTION WHEN others THEN
  RETURN NULL;
END
$$;

REVOKE ALL ON FUNCTION gov_policy.policy_publication_record_v12b(
  text,uuid,text,uuid,uuid,text,text,text,text,uuid,text,text
) FROM PUBLIC;

DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['ak_admin','ak_admin_api','ak_go_worker','ak_worker','ak_exec'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname=r) THEN
      EXECUTE format(
        'GRANT EXECUTE ON FUNCTION gov_policy.policy_publication_record_v12b(text,uuid,text,uuid,uuid,text,text,text,text,uuid,text,text) TO %I',
        r
      );
    END IF;
  END LOOP;
END
$$;

COMMIT;