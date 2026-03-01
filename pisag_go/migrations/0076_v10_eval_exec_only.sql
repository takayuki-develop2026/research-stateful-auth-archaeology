BEGIN;

-- create evaluation row (queued)
CREATE OR REPLACE FUNCTION agent_v10.evaluation_create_v10(
  p_project_id varchar,
  p_proposal_id uuid,
  p_evaluation_type varchar,
  p_dataset_evidence_ref uuid,
  p_metrics_evidence_ref uuid,
  p_metrics_summary jsonb,
  p_guard_summary jsonb,
  p_status varchar,
  p_trace_id varchar
) RETURNS uuid
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = agent_v10, pg_catalog
AS $$
DECLARE v_id uuid;
BEGIN
  INSERT INTO agent_v10.proposal_evaluations_v10(
    project_id, proposal_id, evaluation_type,
    dataset_evidence_ref, metrics_evidence_ref,
    metrics_summary, guard_summary,
    status, trace_id,
    started_at, finished_at
  )
  VALUES(
    p_project_id, p_proposal_id, p_evaluation_type,
    p_dataset_evidence_ref, p_metrics_evidence_ref,
    COALESCE(p_metrics_summary,'{}'::jsonb), COALESCE(p_guard_summary,'{}'::jsonb),
    p_status, p_trace_id,
    now(), now()
  )
  RETURNING id INTO v_id;

  RETURN v_id;
EXCEPTION WHEN others THEN
  RETURN NULL;
END
$$;

-- get evaluation
CREATE OR REPLACE FUNCTION agent_v10.evaluation_get_v10(
  p_project_id varchar,
  p_evaluation_id uuid
) RETURNS SETOF agent_v10.proposal_evaluations_v10
LANGUAGE sql
SECURITY DEFINER
SET search_path = agent_v10, pg_catalog
AS $$
  SELECT * FROM agent_v10.proposal_evaluations_v10
   WHERE project_id = p_project_id AND id = p_evaluation_id
$$;

REVOKE ALL ON FUNCTION agent_v10.evaluation_create_v10(
  varchar,uuid,varchar,uuid,uuid,jsonb,jsonb,varchar,varchar
) FROM PUBLIC;
REVOKE ALL ON FUNCTION agent_v10.evaluation_get_v10(varchar,uuid) FROM PUBLIC;

DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['ak_admin','ak_admin_api','ak_go_worker','ak_worker','ak_exec'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname=r) THEN
      EXECUTE format('GRANT EXECUTE ON FUNCTION agent_v10.evaluation_create_v10(varchar,uuid,varchar,uuid,uuid,jsonb,jsonb,varchar,varchar) TO %I', r);
      EXECUTE format('GRANT EXECUTE ON FUNCTION agent_v10.evaluation_get_v10(varchar,uuid) TO %I', r);
    END IF;
  END LOOP;
END
$$;

COMMIT;