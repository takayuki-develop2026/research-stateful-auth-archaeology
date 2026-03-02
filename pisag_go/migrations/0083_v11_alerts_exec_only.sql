BEGIN;

-- Tighten schema
REVOKE ALL ON ALL TABLES IN SCHEMA ops_v11 FROM PUBLIC;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA ops_v11 FROM PUBLIC;

-- -------------------------
-- notify_channels_v11
-- -------------------------
CREATE OR REPLACE FUNCTION ops_v11.notify_channel_upsert_v11(
  p_project_id varchar,
  p_channel_key text,
  p_channel_type varchar,
  p_destination_ref text,
  p_status varchar
) RETURNS bigint
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
DECLARE v_id bigint;
BEGIN
  INSERT INTO ops_v11.notify_channels_v11(project_id, channel_key, channel_type, destination_ref, status)
  VALUES(p_project_id, p_channel_key, p_channel_type, p_destination_ref, COALESCE(p_status,'active'))
  ON CONFLICT (project_id, channel_key)
  DO UPDATE SET
    channel_type = EXCLUDED.channel_type,
    destination_ref = EXCLUDED.destination_ref,
    status = EXCLUDED.status,
    updated_at = now()
  RETURNING id INTO v_id;
  RETURN v_id;
EXCEPTION WHEN others THEN
  RETURN NULL;
END $$;

CREATE OR REPLACE FUNCTION ops_v11.notify_channel_list_v11(
  p_project_id varchar,
  p_status varchar,
  p_limit int,
  p_offset int
) RETURNS SETOF ops_v11.notify_channels_v11
LANGUAGE sql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
  SELECT *
    FROM ops_v11.notify_channels_v11
   WHERE project_id = p_project_id
     AND (p_status IS NULL OR lower(status)=lower(p_status))
   ORDER BY updated_at DESC
   LIMIT LEAST(COALESCE(p_limit,50),200)
  OFFSET GREATEST(COALESCE(p_offset,0),0)
$$;

CREATE OR REPLACE FUNCTION ops_v11.notify_channel_get_v11(
  p_project_id varchar,
  p_channel_id bigint
) RETURNS SETOF ops_v11.notify_channels_v11
LANGUAGE sql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
  SELECT * FROM ops_v11.notify_channels_v11
   WHERE project_id=p_project_id AND id=p_channel_id
$$;

CREATE OR REPLACE FUNCTION ops_v11.notify_channel_set_status_v11(
  p_project_id varchar,
  p_channel_id bigint,
  p_status varchar
) RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
BEGIN
  UPDATE ops_v11.notify_channels_v11
     SET status=p_status, updated_at=now()
   WHERE project_id=p_project_id AND id=p_channel_id;
  RETURN FOUND;
EXCEPTION WHEN others THEN
  RETURN false;
END $$;

-- -------------------------
-- alert_rules_v11
-- -------------------------
CREATE OR REPLACE FUNCTION ops_v11.alert_rule_upsert_v11(
  p_project_id varchar,
  p_rule_key text,
  p_severity varchar,
  p_status varchar,
  p_condition_evidence_asset_id bigint,
  p_dedupe_key_template text,
  p_cooldown_seconds int,
  p_notify_channel_ids bigint[]
) RETURNS bigint
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
DECLARE v_id bigint;
BEGIN
  INSERT INTO ops_v11.alert_rules_v11(
    project_id, rule_key, severity, status,
    condition_evidence_asset_id, dedupe_key_template, cooldown_seconds, notify_channel_ids
  )
  VALUES(
    p_project_id, p_rule_key, p_severity, COALESCE(p_status,'active'),
    p_condition_evidence_asset_id, p_dedupe_key_template, COALESCE(p_cooldown_seconds,300),
    COALESCE(p_notify_channel_ids, ARRAY[]::bigint[])
  )
  ON CONFLICT (project_id, rule_key)
  DO UPDATE SET
    severity = EXCLUDED.severity,
    status = EXCLUDED.status,
    condition_evidence_asset_id = EXCLUDED.condition_evidence_asset_id,
    dedupe_key_template = EXCLUDED.dedupe_key_template,
    cooldown_seconds = EXCLUDED.cooldown_seconds,
    notify_channel_ids = EXCLUDED.notify_channel_ids,
    updated_at = now()
  RETURNING id INTO v_id;
  RETURN v_id;
EXCEPTION WHEN others THEN
  RETURN NULL;
END $$;

CREATE OR REPLACE FUNCTION ops_v11.alert_rule_list_v11(
  p_project_id varchar,
  p_status varchar,
  p_limit int,
  p_offset int
) RETURNS SETOF ops_v11.alert_rules_v11
LANGUAGE sql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
  SELECT *
    FROM ops_v11.alert_rules_v11
   WHERE project_id = p_project_id
     AND (p_status IS NULL OR lower(status)=lower(p_status))
   ORDER BY updated_at DESC
   LIMIT LEAST(COALESCE(p_limit,50),200)
  OFFSET GREATEST(COALESCE(p_offset,0),0)
$$;

CREATE OR REPLACE FUNCTION ops_v11.alert_rule_get_v11(
  p_project_id varchar,
  p_rule_id bigint
) RETURNS SETOF ops_v11.alert_rules_v11
LANGUAGE sql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
  SELECT * FROM ops_v11.alert_rules_v11
   WHERE project_id=p_project_id AND id=p_rule_id
$$;

CREATE OR REPLACE FUNCTION ops_v11.alert_rule_set_status_v11(
  p_project_id varchar,
  p_rule_id bigint,
  p_status varchar
) RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
BEGIN
  UPDATE ops_v11.alert_rules_v11
     SET status=p_status, updated_at=now()
   WHERE project_id=p_project_id AND id=p_rule_id;
  RETURN FOUND;
EXCEPTION WHEN others THEN
  RETURN false;
END $$;

-- -------------------------
-- alerts_v11 (fire/ack/resolve/list/get)
-- fire does dedupe/cooldown: if same dedupe_key fired within cooldown and still open/ack => return existing
-- -------------------------
CREATE OR REPLACE FUNCTION ops_v11.alert_fire_v11(
  p_project_id varchar,
  p_rule_id bigint,
  p_dedupe_key text,
  p_trace_id text,
  p_run_id uuid,
  p_policy_set_id uuid,
  p_policy_version_id uuid,
  p_provider_hint varchar,
  p_context_evidence_asset_id bigint,
  p_related_evidence_asset_ids bigint[]
) RETURNS TABLE(alert_id bigint, found_existing boolean)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
DECLARE
  v_cooldown int;
  v_now timestamptz := now();
  v_existing bigint;
  v_sev varchar;
BEGIN
  SELECT cooldown_seconds, severity
    INTO v_cooldown, v_sev
    FROM ops_v11.alert_rules_v11
   WHERE project_id=p_project_id AND id=p_rule_id AND lower(status)='active';

  IF v_cooldown IS NULL THEN
    -- rule missing or paused
    RETURN QUERY SELECT NULL::bigint, false;
    RETURN;
  END IF;

  SELECT a.id
    INTO v_existing
    FROM ops_v11.alerts_v11 a
   WHERE a.project_id=p_project_id
     AND a.dedupe_key=p_dedupe_key
     AND lower(a.status) IN ('open','acknowledged')
     AND a.fired_at_utc >= (v_now - make_interval(secs => v_cooldown))
   ORDER BY a.fired_at_utc DESC
   LIMIT 1;

  IF v_existing IS NOT NULL THEN
    RETURN QUERY SELECT v_existing, true;
    RETURN;
  END IF;

  INSERT INTO ops_v11.alerts_v11(
    project_id, rule_id, severity, status,
    fired_at_utc, dedupe_key,
    trace_id, run_id, policy_set_id, policy_version_id, provider_hint,
    context_evidence_asset_id, related_evidence_asset_ids
  )
  VALUES(
    p_project_id, p_rule_id, v_sev, 'open',
    v_now, p_dedupe_key,
    p_trace_id, p_run_id, p_policy_set_id, p_policy_version_id, p_provider_hint,
    p_context_evidence_asset_id, COALESCE(p_related_evidence_asset_ids, ARRAY[]::bigint[])
  )
  RETURNING id INTO v_existing;

  RETURN QUERY SELECT v_existing, false;
END $$;

CREATE OR REPLACE FUNCTION ops_v11.alert_get_v11(
  p_project_id varchar,
  p_alert_id bigint
) RETURNS SETOF ops_v11.alerts_v11
LANGUAGE sql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
  SELECT * FROM ops_v11.alerts_v11
   WHERE project_id=p_project_id AND id=p_alert_id
$$;

CREATE OR REPLACE FUNCTION ops_v11.alert_list_v11(
  p_project_id varchar,
  p_status varchar,
  p_severity varchar,
  p_limit int,
  p_offset int
) RETURNS SETOF ops_v11.alerts_v11
LANGUAGE sql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
  SELECT *
    FROM ops_v11.alerts_v11
   WHERE project_id=p_project_id
     AND (p_status IS NULL OR lower(status)=lower(p_status))
     AND (p_severity IS NULL OR lower(severity)=lower(p_severity))
   ORDER BY fired_at_utc DESC
   LIMIT LEAST(COALESCE(p_limit,50),200)
  OFFSET GREATEST(COALESCE(p_offset,0),0)
$$;

CREATE OR REPLACE FUNCTION ops_v11.alert_ack_v11(
  p_project_id varchar,
  p_alert_id bigint
) RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
BEGIN
  UPDATE ops_v11.alerts_v11
     SET status='acknowledged', updated_at=now()
   WHERE project_id=p_project_id AND id=p_alert_id AND lower(status)='open';
  RETURN FOUND;
EXCEPTION WHEN others THEN
  RETURN false;
END $$;

CREATE OR REPLACE FUNCTION ops_v11.alert_resolve_v11(
  p_project_id varchar,
  p_alert_id bigint
) RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ops_v11, pg_catalog
AS $$
BEGIN
  UPDATE ops_v11.alerts_v11
     SET status='resolved', resolved_at_utc=now(), updated_at=now()
   WHERE project_id=p_project_id AND id=p_alert_id AND lower(status) IN ('open','acknowledged');
  RETURN FOUND;
EXCEPTION WHEN others THEN
  RETURN false;
END $$;

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
END $$;

COMMIT;