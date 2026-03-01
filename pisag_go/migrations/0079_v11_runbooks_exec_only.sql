BEGIN;

REVOKE ALL ON ALL TABLES IN SCHEMA ops_v11 FROM PUBLIC;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA ops_v11 FROM PUBLIC;

-- upsert runbook by (project_id, runbook_key)
CREATE OR REPLACE FUNCTION ops_v11.runbook_upsert_v11(
  p_project_id varchar,
  p_runbook_key varchar,
  p_title varchar,
  p_steps_evidence_ref uuid,
  p_safety_checks_evidence_ref uuid,
  p_required_roles text[],
  p_status varchar,
  p_created_by_type varchar,
  p_created_by_id varchar
) RETURNS uuid
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
DECLARE v_id uuid;
BEGIN
  INSERT INTO ops_v11.runbooks_v11(
    project_id, runbook_key, title,
    steps_evidence_ref, safety_checks_evidence_ref,
    required_roles, status,
    created_by_type, created_by_id
  )
  VALUES(
    p_project_id, p_runbook_key, p_title,
    p_steps_evidence_ref, p_safety_checks_evidence_ref,
    COALESCE(p_required_roles, ARRAY[]::text[]),
    COALESCE(p_status, 'active'),
    COALESCE(p_created_by_type,'system'),
    p_created_by_id
  )
  ON CONFLICT (project_id, runbook_key)
  DO UPDATE SET
    title = EXCLUDED.title,
    steps_evidence_ref = EXCLUDED.steps_evidence_ref,
    safety_checks_evidence_ref = EXCLUDED.safety_checks_evidence_ref,
    required_roles = EXCLUDED.required_roles,
    status = EXCLUDED.status,
    updated_at = now()
  RETURNING id INTO v_id;

  RETURN v_id;
EXCEPTION WHEN others THEN
  RETURN NULL;
END
$$;

-- list runbooks
CREATE OR REPLACE FUNCTION ops_v11.runbook_list_v11(
  p_project_id varchar,
  p_status varchar,
  p_limit int,
  p_offset int
) RETURNS SETOF ops_v11.runbooks_v11
LANGUAGE sql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
  SELECT * FROM ops_v11.runbooks_v11
   WHERE project_id = p_project_id
     AND (p_status IS NULL OR status = p_status)
   ORDER BY updated_at DESC
   LIMIT LEAST(COALESCE(p_limit,50),200)
  OFFSET GREATEST(COALESCE(p_offset,0),0)
$$;

-- get runbook
CREATE OR REPLACE FUNCTION ops_v11.runbook_get_v11(
  p_project_id varchar,
  p_runbook_id uuid
) RETURNS SETOF ops_v11.runbooks_v11
LANGUAGE sql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
  SELECT * FROM ops_v11.runbooks_v11
   WHERE project_id = p_project_id AND id = p_runbook_id
$$;

REVOKE ALL ON ALL FUNCTIONS IN SCHEMA ops_v11 FROM PUBLIC;

DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['ak_admin','ak_admin_api','ak_go_worker','ak_worker','ak_exec'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname=r) THEN
      EXECUTE format('GRANT USAGE ON SCHEMA ops_v11 TO %I', r);
      EXECUTE format('GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA ops_v11 TO %I', r);
    END IF;
  END LOOP;
END
$$;

COMMIT;