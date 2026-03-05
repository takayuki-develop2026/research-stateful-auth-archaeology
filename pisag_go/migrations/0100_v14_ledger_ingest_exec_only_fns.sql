-- 0100_v14_ledger_ingest_exec_only_fns.sql
-- v14.1 exec-only functions for ingest orchestration

BEGIN;

-- Helper already exists from 0097, but keep idempotent in case of ordering differences.
CREATE OR REPLACE FUNCTION _jsonb_is_array(p jsonb)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
AS $$
  SELECT COALESCE(jsonb_typeof(p) = 'array', false);
$$;

-- =========================================================
-- Accept (idempotent): creates ingest_run or returns existing by (project_id, idempotency_key)
-- =========================================================
CREATE OR REPLACE FUNCTION ledger_ingest_run_accept_v14(
  p_project_id text,
  p_mode ledger_ingest_mode_v14,
  p_source_event_key text,
  p_from_ts timestamptz,
  p_to_ts timestamptz,
  p_filter jsonb,
  p_idempotency_key text,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE (ingest_run_id uuid, status text)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_id uuid;
BEGIN
  IF p_project_id IS NULL OR length(trim(p_project_id)) = 0 THEN
    RAISE EXCEPTION 'project_id is required';
  END IF;
  IF p_idempotency_key IS NULL OR length(trim(p_idempotency_key)) = 0 THEN
    RAISE EXCEPTION 'idempotency_key is required';
  END IF;
  IF p_run_id IS NULL OR length(trim(p_run_id)) = 0 THEN
    RAISE EXCEPTION 'run_id is required';
  END IF;
  IF p_trace_id IS NULL OR length(trim(p_trace_id)) = 0 THEN
    RAISE EXCEPTION 'trace_id is required';
  END IF;
  IF p_policy_version_id IS NULL OR length(trim(p_policy_version_id)) = 0 THEN
    RAISE EXCEPTION 'policy_version_id is required';
  END IF;
  IF p_filter IS NULL THEN
    p_filter := '{}'::jsonb;
  END IF;
  IF jsonb_typeof(p_filter) <> 'object' THEN
    RAISE EXCEPTION 'filter must be a json object';
  END IF;
  IF NOT _jsonb_is_array(p_evidence_refs) THEN
    RAISE EXCEPTION 'evidence_refs must be a json array';
  END IF;

  -- Validate mode scope
  IF p_mode = 'single_event' THEN
    IF p_source_event_key IS NULL OR length(trim(p_source_event_key)) = 0 THEN
      RAISE EXCEPTION 'source_event_key is required for single_event';
    END IF;
    IF p_from_ts IS NOT NULL OR p_to_ts IS NOT NULL THEN
      RAISE EXCEPTION 'from_ts/to_ts must be null for single_event';
    END IF;
  ELSIF p_mode = 'range' THEN
    IF p_source_event_key IS NOT NULL THEN
      RAISE EXCEPTION 'source_event_key must be null for range';
    END IF;
    IF p_from_ts IS NULL OR p_to_ts IS NULL OR NOT (p_from_ts < p_to_ts) THEN
      RAISE EXCEPTION 'valid from_ts/to_ts required for range';
    END IF;
  ELSE
    RAISE EXCEPTION 'invalid mode';
  END IF;

  INSERT INTO ledger_ingest_runs(
    project_id, mode, source_event_key, from_ts, to_ts, filter,
    idempotency_key, status, run_id, trace_id, policy_version_id, evidence_refs
  )
  VALUES (
    p_project_id, p_mode, p_source_event_key, p_from_ts, p_to_ts, p_filter,
    p_idempotency_key, 'accepted', p_run_id, p_trace_id, p_policy_version_id, p_evidence_refs
  )
  ON CONFLICT (project_id, idempotency_key) DO NOTHING
  RETURNING id INTO v_id;

  IF v_id IS NOT NULL THEN
    ingest_run_id := v_id;
    status := 'accepted_created';
    RETURN NEXT;
    RETURN;
  END IF;

  SELECT lir.id INTO v_id
  FROM ledger_ingest_runs lir
  WHERE lir.project_id = p_project_id AND lir.idempotency_key = p_idempotency_key;

  ingest_run_id := v_id;
  status := 'accepted_exists';
  RETURN NEXT;
END;
$$;

-- =========================================================
-- Claim next: move accepted -> running (single-owner)
-- Returns one row or zero rows.
-- =========================================================
CREATE OR REPLACE FUNCTION ledger_ingest_run_claim_next_v14(
  p_project_id text
)
RETURNS TABLE (
  ingest_run_id uuid,
  mode ledger_ingest_mode_v14,
  source_event_key text,
  from_ts timestamptz,
  to_ts timestamptz,
  filter jsonb,
  idempotency_key text,
  run_id text,
  trace_id text,
  policy_version_id text
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF p_project_id IS NULL OR length(trim(p_project_id)) = 0 THEN
    RAISE EXCEPTION 'project_id is required';
  END IF;

  RETURN QUERY
  WITH cte AS (
    SELECT id
    FROM ledger_ingest_runs
    WHERE project_id = p_project_id
      AND status = 'accepted'
    ORDER BY created_at ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
  )
  UPDATE ledger_ingest_runs lir
     SET status = 'running',
         updated_at = now()
    FROM cte
   WHERE lir.id = cte.id
  RETURNING
    lir.id,
    lir.mode,
    lir.source_event_key,
    lir.from_ts,
    lir.to_ts,
    lir.filter,
    lir.idempotency_key,
    lir.run_id,
    lir.trace_id,
    lir.policy_version_id;
END;
$$;

-- =========================================================
-- Touch (heartbeat)
-- =========================================================
CREATE OR REPLACE FUNCTION ledger_ingest_run_touch_v14(
  p_ingest_run_id uuid
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF p_ingest_run_id IS NULL THEN
    RAISE EXCEPTION 'ingest_run_id is required';
  END IF;

  UPDATE ledger_ingest_runs
     SET updated_at = now()
   WHERE id = p_ingest_run_id;
END;
$$;

-- =========================================================
-- Mark succeeded (records stats + evidence_refs)
-- =========================================================
CREATE OR REPLACE FUNCTION ledger_ingest_run_mark_succeeded_v14(
  p_ingest_run_id uuid,
  p_stats jsonb DEFAULT '{}'::jsonb,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_existing_refs jsonb;
  v_new_refs jsonb;
BEGIN
  IF p_ingest_run_id IS NULL THEN
    RAISE EXCEPTION 'ingest_run_id is required';
  END IF;
  IF p_stats IS NULL THEN
    p_stats := '{}'::jsonb;
  END IF;
  IF jsonb_typeof(p_stats) <> 'object' THEN
    RAISE EXCEPTION 'stats must be a json object';
  END IF;
  IF NOT _jsonb_is_array(p_append_evidence_refs) THEN
    RAISE EXCEPTION 'append_evidence_refs must be a json array';
  END IF;

  SELECT evidence_refs INTO v_existing_refs
  FROM ledger_ingest_runs
  WHERE id = p_ingest_run_id
  FOR UPDATE;

  v_new_refs := COALESCE(v_existing_refs, '[]'::jsonb) || p_append_evidence_refs;

  UPDATE ledger_ingest_runs
     SET status = 'succeeded',
         stats = p_stats,
         evidence_refs = v_new_refs,
         updated_at = now()
   WHERE id = p_ingest_run_id;
END;
$$;

-- =========================================================
-- Mark failed_recorded (records stats + evidence_refs)
-- =========================================================
CREATE OR REPLACE FUNCTION ledger_ingest_run_mark_failed_recorded_v14(
  p_ingest_run_id uuid,
  p_stats jsonb DEFAULT '{}'::jsonb,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_existing_refs jsonb;
  v_new_refs jsonb;
BEGIN
  IF p_ingest_run_id IS NULL THEN
    RAISE EXCEPTION 'ingest_run_id is required';
  END IF;
  IF p_stats IS NULL THEN
    p_stats := '{}'::jsonb;
  END IF;
  IF jsonb_typeof(p_stats) <> 'object' THEN
    RAISE EXCEPTION 'stats must be a json object';
  END IF;
  IF NOT _jsonb_is_array(p_append_evidence_refs) THEN
    RAISE EXCEPTION 'append_evidence_refs must be a json array';
  END IF;

  SELECT evidence_refs INTO v_existing_refs
  FROM ledger_ingest_runs
  WHERE id = p_ingest_run_id
  FOR UPDATE;

  v_new_refs := COALESCE(v_existing_refs, '[]'::jsonb) || p_append_evidence_refs;

  UPDATE ledger_ingest_runs
     SET status = 'failed_recorded',
         stats = p_stats,
         evidence_refs = v_new_refs,
         updated_at = now()
   WHERE id = p_ingest_run_id;
END;
$$;

-- =========================================================
-- SECURITY: revoke from PUBLIC (fail-closed)
-- =========================================================
REVOKE ALL ON FUNCTION ledger_ingest_run_accept_v14(
  text, ledger_ingest_mode_v14, text, timestamptz, timestamptz, jsonb, text, text, text, text, jsonb
) FROM PUBLIC;

REVOKE ALL ON FUNCTION ledger_ingest_run_claim_next_v14(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION ledger_ingest_run_touch_v14(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION ledger_ingest_run_mark_succeeded_v14(uuid, jsonb, jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION ledger_ingest_run_mark_failed_recorded_v14(uuid, jsonb, jsonb) FROM PUBLIC;

COMMIT;