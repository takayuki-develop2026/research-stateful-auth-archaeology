BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ------------------------------------------------------------
-- policy_decision_append_v21 (EXECUTE ONLY)
-- ------------------------------------------------------------
CREATE OR REPLACE FUNCTION public.policy_decision_append_v21(
  p_project_id varchar,
  p_decision_key text,
  p_trace_id text,
  p_run_id uuid,
  p_subject_type text,
  p_subject_id text,
  p_action_key text,
  p_action_class text,
  p_policy_version_str text,
  p_result text,
  p_input_hash_sha256 text,
  p_decision_input_evidence_asset_id bigint,
  p_decision_result_evidence_asset_id bigint,
  p_resource_evidence_asset_id bigint,
  p_obligations_evidence_asset_id bigint,
  p_reason_codes_evidence_asset_id bigint
)
RETURNS TABLE(decision_id bigint, found_existing boolean)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id varchar(26);
  v_key char(64);
BEGIN
  v_project_id := btrim(coalesce(p_project_id::text,''))::varchar(26);
  IF v_project_id = '' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;

  v_key := btrim(coalesce(p_decision_key::text,''))::char(64);
  IF length(v_key::text) <> 64 THEN RAISE EXCEPTION 'decision_key must be 64 hex' USING ERRCODE='22023'; END IF;

  SELECT id INTO decision_id
  FROM public.policy_decisions_v21
  WHERE project_id=v_project_id AND decision_key=v_key
  LIMIT 1;

  IF decision_id IS NOT NULL THEN
    found_existing := true;
    RETURN NEXT;
    RETURN;
  END IF;

  INSERT INTO public.policy_decisions_v21(
    project_id, decision_key, trace_id, run_id, decision_time_utc,
    subject_type, subject_id,
    action_key, action_class,
    policy_version_str, result,
    input_hash_sha256,
    decision_input_evidence_asset_id,
    decision_result_evidence_asset_id,
    resource_evidence_asset_id,
    obligations_evidence_asset_id,
    reason_codes_evidence_asset_id
  )
  VALUES (
    v_project_id,
    v_key,
    btrim(coalesce(p_trace_id,'')),
    p_run_id,
    now(),
    btrim(coalesce(p_subject_type,'')),
    btrim(coalesce(p_subject_id,'')),
    btrim(coalesce(p_action_key,'')),
    btrim(coalesce(p_action_class,'')),
    btrim(coalesce(p_policy_version_str,'')),
    btrim(coalesce(p_result,'')),
    btrim(coalesce(p_input_hash_sha256,'')),
    p_decision_input_evidence_asset_id,
    p_decision_result_evidence_asset_id,
    p_resource_evidence_asset_id,
    p_obligations_evidence_asset_id,
    p_reason_codes_evidence_asset_id
  )
  RETURNING id INTO decision_id;

  found_existing := false;
  RETURN NEXT;
END;
$$;

-- ------------------------------------------------------------
-- compliance_event_append_v21 (EXECUTE ONLY)
-- ------------------------------------------------------------
CREATE OR REPLACE FUNCTION public.compliance_event_append_v21(
  p_project_id varchar,
  p_trace_id text,
  p_event_type text,
  p_event_evidence_asset_id bigint,
  p_primary_artifact_asset_id bigint
)
RETURNS TABLE(event_id bigint)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id varchar(26);
BEGIN
  v_project_id := btrim(coalesce(p_project_id::text,''))::varchar(26);
  IF v_project_id = '' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;

  INSERT INTO public.compliance_events_v21(
    project_id, trace_id, event_type, event_evidence_asset_id, primary_artifact_asset_id, created_at_utc
  )
  VALUES (
    v_project_id,
    btrim(coalesce(p_trace_id,'')),
    btrim(coalesce(p_event_type,'')),
    p_event_evidence_asset_id,
    p_primary_artifact_asset_id,
    now()
  )
  RETURNING id INTO event_id;

  RETURN NEXT;
END;
$$;

-- ------------------------------------------------------------
-- api_key_create_v21 / revoke_v21 (EXECUTE ONLY)
-- key material is NOT stored; caller provides key_hash + key_id (public).
-- ------------------------------------------------------------
CREATE OR REPLACE FUNCTION public.api_key_create_v21(
  p_project_id varchar,
  p_key_id varchar,
  p_key_hash text,
  p_scope_evidence_asset_id bigint,
  p_expires_at_utc timestamptz,
  p_created_by_type varchar,
  p_created_by_id varchar
)
RETURNS TABLE(api_key_row_id bigint)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id varchar(26);
  v_key_hash char(64);
BEGIN
  v_project_id := btrim(coalesce(p_project_id::text,''))::varchar(26);
  IF v_project_id = '' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;

  v_key_hash := btrim(coalesce(p_key_hash::text,''))::char(64);
  IF length(v_key_hash::text) <> 64 THEN RAISE EXCEPTION 'key_hash must be 64 hex' USING ERRCODE='22023'; END IF;

  INSERT INTO public.api_keys_v21(
    project_id, key_id, key_hash, scope_evidence_asset_id,
    status, expires_at_utc, created_by_type, created_by_id, created_at_utc
  )
  VALUES (
    v_project_id,
    btrim(coalesce(p_key_id::text,''))::varchar(64),
    v_key_hash,
    p_scope_evidence_asset_id,
    'active',
    p_expires_at_utc,
    lower(btrim(coalesce(p_created_by_type::text,'')))::varchar(16),
    NULLIF(btrim(coalesce(p_created_by_id::text,'')),'')::varchar(128),
    now()
  )
  RETURNING id INTO api_key_row_id;

  RETURN NEXT;
END;
$$;

CREATE OR REPLACE FUNCTION public.api_key_revoke_v21(
  p_project_id varchar,
  p_key_id varchar,
  p_revoked_reason_evidence_asset_id bigint,
  p_actor_type varchar,
  p_actor_id varchar
)
RETURNS TABLE(updated boolean)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id varchar(26);
BEGIN
  v_project_id := btrim(coalesce(p_project_id::text,''))::varchar(26);
  UPDATE public.api_keys_v21
  SET status='revoked',
      revoked_at_utc=now(),
      revoked_reason_evidence_asset_id=p_revoked_reason_evidence_asset_id
  WHERE project_id=v_project_id
    AND key_id=btrim(coalesce(p_key_id::text,''))::varchar(64)
    AND status='active';

  updated := (FOUND);
  RETURN NEXT;
END;
$$;

-- ------------------------------------------------------------
-- privilege_grant / revoke (EXECUTE ONLY)
-- ------------------------------------------------------------
CREATE OR REPLACE FUNCTION public.privilege_grant_v21(
  p_project_id varchar,
  p_subject_type text,
  p_subject_id text,
  p_granted_role text,
  p_scope_evidence_asset_id bigint,
  p_grant_reason_evidence_asset_id bigint,
  p_granted_by_user_id text
)
RETURNS TABLE(grant_id bigint)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id varchar(26);
BEGIN
  v_project_id := btrim(coalesce(p_project_id::text,''))::varchar(26);

  INSERT INTO public.privilege_grants_v21(
    project_id, subject_type, subject_id, granted_role,
    scope_evidence_asset_id, grant_reason_evidence_asset_id,
    granted_by_user_id, granted_at_utc
  )
  VALUES (
    v_project_id,
    btrim(coalesce(p_subject_type,'')),
    btrim(coalesce(p_subject_id,'')),
    btrim(coalesce(p_granted_role,'')),
    p_scope_evidence_asset_id,
    p_grant_reason_evidence_asset_id,
    btrim(coalesce(p_granted_by_user_id,'')),
    now()
  )
  RETURNING id INTO grant_id;

  RETURN NEXT;
END;
$$;

CREATE OR REPLACE FUNCTION public.privilege_revoke_v21(
  p_project_id varchar,
  p_grant_id bigint,
  p_revoked_by_user_id text,
  p_revoke_reason_evidence_asset_id bigint
)
RETURNS TABLE(updated boolean)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id varchar(26);
BEGIN
  v_project_id := btrim(coalesce(p_project_id::text,''))::varchar(26);

  UPDATE public.privilege_grants_v21
  SET revoked_at_utc=now(),
      revoked_by_user_id=btrim(coalesce(p_revoked_by_user_id,'')),
      revoke_reason_evidence_asset_id=p_revoke_reason_evidence_asset_id
  WHERE project_id=v_project_id
    AND id=p_grant_id
    AND revoked_at_utc IS NULL;

  updated := (FOUND);
  RETURN NEXT;
END;
$$;

-- ------------------------------------------------------------
-- key_rotation_plan helpers (EXECUTE ONLY)
-- ------------------------------------------------------------
CREATE OR REPLACE FUNCTION public.key_rotation_plan_create_v21(
  p_project_id varchar,
  p_rotation_key text,
  p_key_domain text,
  p_plan_evidence_asset_id bigint,
  p_created_by_user_id text
)
RETURNS TABLE(rotation_plan_id bigint, found_existing boolean)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id varchar(26);
  v_key char(64);
BEGIN
  v_project_id := btrim(coalesce(p_project_id::text,''))::varchar(26);
  v_key := btrim(coalesce(p_rotation_key::text,''))::char(64);

  SELECT id INTO rotation_plan_id
  FROM public.key_rotation_plans_v21
  WHERE project_id=v_project_id AND rotation_key=v_key
  LIMIT 1;

  IF rotation_plan_id IS NOT NULL THEN
    found_existing := true;
    RETURN NEXT;
    RETURN;
  END IF;

  INSERT INTO public.key_rotation_plans_v21(
    project_id, rotation_key, key_domain, status,
    plan_evidence_asset_id, created_by_user_id, created_at_utc
  )
  VALUES (
    v_project_id, v_key, btrim(coalesce(p_key_domain,'')), 'planned',
    p_plan_evidence_asset_id, btrim(coalesce(p_created_by_user_id,'')), now()
  )
  RETURNING id INTO rotation_plan_id;

  found_existing := false;
  RETURN NEXT;
END;
$$;

CREATE OR REPLACE FUNCTION public.key_rotation_plan_mark_v21(
  p_project_id varchar,
  p_rotation_plan_id bigint,
  p_status text,
  p_verification_evidence_asset_id bigint
)
RETURNS TABLE(updated boolean)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id varchar(26);
  v_status text;
BEGIN
  v_project_id := btrim(coalesce(p_project_id::text,''))::varchar(26);
  v_status := btrim(coalesce(p_status,''));

  IF v_status NOT IN ('planned','in_progress','verified','completed','aborted') THEN
    RAISE EXCEPTION 'invalid status %', v_status USING ERRCODE='22023';
  END IF;

  UPDATE public.key_rotation_plans_v21
  SET status=v_status,
      verification_evidence_asset_id=COALESCE(p_verification_evidence_asset_id, verification_evidence_asset_id),
      completed_at_utc=CASE WHEN v_status='completed' THEN now() ELSE completed_at_utc END
  WHERE project_id=v_project_id AND id=p_rotation_plan_id;

  updated := (FOUND);
  RETURN NEXT;
END;
$$;

COMMIT;