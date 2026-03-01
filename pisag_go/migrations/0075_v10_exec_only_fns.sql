BEGIN;

REVOKE ALL ON ALL TABLES IN SCHEMA agent_v10 FROM PUBLIC;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA agent_v10 FROM PUBLIC;

-- create proposal
CREATE OR REPLACE FUNCTION agent_v10.proposal_create_v10(
  p_project_id varchar,
  p_policy_set_id uuid,
  p_policy_version_base uuid,
  p_proposal_type varchar,
  p_risk_level varchar,
  p_change_set_evidence_ref uuid,
  p_rationale_summary varchar,
  p_rationale_evidence_ref uuid,
  p_impact_summary jsonb,
  p_created_by_type varchar,
  p_created_by_id varchar,
  p_trace_id varchar,
  p_idempotency_key text
) RETURNS uuid
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = agent_v10, pg_catalog
AS $$
DECLARE v_id uuid;
BEGIN
  -- idempotency: if you want strict idempotency, add a table; for v10.0 we keep simple.
  INSERT INTO agent_v10.routing_proposals_v10(
    project_id, policy_set_id, policy_version_base,
    proposal_type, risk_level,
    change_set_evidence_ref, rationale_summary, rationale_evidence_ref,
    impact_summary, status,
    created_by_type, created_by_id
  )
  VALUES(
    p_project_id, p_policy_set_id, p_policy_version_base,
    p_proposal_type, p_risk_level,
    p_change_set_evidence_ref, p_rationale_summary, p_rationale_evidence_ref,
    COALESCE(p_impact_summary,'{}'::jsonb), 'draft',
    p_created_by_type, p_created_by_id
  )
  RETURNING id INTO v_id;

  RETURN v_id;
EXCEPTION WHEN others THEN
  RETURN NULL;
END
$$;

-- get proposal
CREATE OR REPLACE FUNCTION agent_v10.proposal_get_v10(
  p_project_id varchar,
  p_proposal_id uuid
) RETURNS SETOF agent_v10.routing_proposals_v10
LANGUAGE sql
SECURITY DEFINER
SET search_path = agent_v10, pg_catalog
AS $$
  SELECT * FROM agent_v10.routing_proposals_v10
   WHERE project_id = p_project_id AND id = p_proposal_id
$$;

-- list proposals
CREATE OR REPLACE FUNCTION agent_v10.proposal_list_v10(
  p_project_id varchar,
  p_status varchar,
  p_limit int,
  p_offset int
) RETURNS SETOF agent_v10.routing_proposals_v10
LANGUAGE sql
SECURITY DEFINER
SET search_path = agent_v10, pg_catalog
AS $$
  SELECT * FROM agent_v10.routing_proposals_v10
   WHERE project_id = p_project_id
     AND (p_status IS NULL OR status = p_status)
   ORDER BY created_at DESC
   LIMIT LEAST(COALESCE(p_limit,50),200)
  OFFSET GREATEST(COALESCE(p_offset,0),0)
$$;

-- update proposal status (legal transitions enforced in app for v10.0)
CREATE OR REPLACE FUNCTION agent_v10.proposal_set_status_v10(
  p_project_id varchar,
  p_proposal_id uuid,
  p_status varchar
) RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = agent_v10, pg_catalog
AS $$
BEGIN
  UPDATE agent_v10.routing_proposals_v10
     SET status = p_status,
         updated_at = now()
   WHERE project_id = p_project_id AND id = p_proposal_id;
  RETURN TRUE;
EXCEPTION WHEN others THEN
  RETURN FALSE;
END
$$;

REVOKE ALL ON ALL FUNCTIONS IN SCHEMA agent_v10 FROM PUBLIC;

DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['ak_admin','ak_admin_api','ak_go_worker','ak_worker','ak_exec'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname=r) THEN
      EXECUTE format('GRANT USAGE ON SCHEMA agent_v10 TO %I', r);
      EXECUTE format('GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA agent_v10 TO %I', r);
    END IF;
  END LOOP;
END
$$;

COMMIT;