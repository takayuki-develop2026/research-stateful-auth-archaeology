BEGIN;

-- 1) claim + enqueue（tick）
-- p_next_map: {"<schedule_id>":"2026-03-05T01:02:03Z", ...}
-- cron計算をGoで行う場合にここへ反映。無い場合は interval or +60s fallback。
CREATE OR REPLACE FUNCTION runsched_tick_enqueue_v19(
  p_project_id VARCHAR,
  p_now_utc TIMESTAMPTZ,
  p_limit INTEGER,
  p_next_map JSONB DEFAULT '{}'::jsonb
)
RETURNS TABLE(
  scheduled_run_id BIGINT,
  schedule_id BIGINT,
  scheduled_for_utc TIMESTAMPTZ,
  trace_id TEXT,
  enqueue_key TEXT,
  status TEXT
)
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
  r RECORD;
  v_trace TEXT;
  v_scheduled_for TIMESTAMPTZ;
  v_enqueue_key TEXT;
  v_next_text TEXT;
  v_next_ts TIMESTAMPTZ;
BEGIN
  -- pick due schedules with row lock (single-owner)
  FOR r IN
    SELECT *
    FROM run_schedules
    WHERE project_id = p_project_id
      AND enabled = TRUE
      AND next_run_at_utc <= p_now_utc
    ORDER BY priority DESC, next_run_at_utc ASC, id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT p_limit
  LOOP
    v_scheduled_for := r.next_run_at_utc;
    v_trace := gen_random_uuid()::text;

    -- enqueue_key = sha256(project|schedule|scheduled_for_utc_rfc3339)
    v_enqueue_key := encode(
      digest(p_project_id || '|' || r.id::text || '|' || to_char(v_scheduled_for AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), 'sha256'),
      'hex'
    );

    -- compute next_run_at_utc_new
    v_next_text := COALESCE(p_next_map ->> (r.id::text), NULL);

    IF v_next_text IS NOT NULL THEN
      v_next_ts := v_next_text::timestamptz;
    ELSE
      IF r.interval_seconds IS NOT NULL THEN
        v_next_ts := r.next_run_at_utc + make_interval(secs => r.interval_seconds);
      ELSE
        -- fallback for cron when no map provided: move by 60s to avoid tight loop
        v_next_ts := date_trunc('minute', p_now_utc) + interval '1 minute';
      END IF;
    END IF;

    -- insert scheduled_runs idempotently: enqueue_key UNIQUE will reject duplicates at DDL layer
    INSERT INTO scheduled_runs(
      project_id, schedule_id, scheduled_for_utc, trace_id, status, enqueue_key
    )
    VALUES (
      p_project_id, r.id, v_scheduled_for, v_trace, 'queued', v_enqueue_key
    )
    ON CONFLICT DO NOTHING;

    -- always move next_run forward once we attempted enqueue (single-owner tx)
    UPDATE run_schedules
      SET last_run_at_utc = v_scheduled_for,
          next_run_at_utc = v_next_ts
    WHERE id = r.id;

    -- return the inserted row (if conflict happened, fetch existing)
    RETURN QUERY
      SELECT sr.id, sr.schedule_id, sr.scheduled_for_utc, sr.trace_id, sr.enqueue_key, sr.status
      FROM scheduled_runs sr
      WHERE sr.project_id = p_project_id
        AND sr.enqueue_key = v_enqueue_key
      LIMIT 1;
  END LOOP;

  RETURN;
END;
$$;

-- 2) dispatch（queued scheduled_runs -> create run -> mark dispatched）
-- budget/policy check results should be computed by Go and applied via separate exec-only fn below.
CREATE OR REPLACE FUNCTION runsched_dispatch_create_run_v19(
  p_project_id VARCHAR,
  p_limit INTEGER,
  p_now_utc TIMESTAMPTZ
)
RETURNS TABLE(
  scheduled_run_id BIGINT,
  run_id TEXT,
  trace_id TEXT,
  status TEXT
)
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
  r RECORD;
  v_run_id TEXT;
  v_input_hash TEXT;
BEGIN
  FOR r IN
    SELECT sr.*, rs.task_type, rs.pipeline_version, rs.policy_version_id, rs.mode,
           rs.input_template_evidence_asset_id, rs.budget_policy_evidence_asset_id, rs.retry_policy_evidence_asset_id
    FROM scheduled_runs sr
    JOIN run_schedules rs ON rs.id = sr.schedule_id
    WHERE sr.project_id = p_project_id
      AND sr.status = 'queued'
    ORDER BY sr.scheduled_for_utc ASC, sr.id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT p_limit
  LOOP
    -- contract existence check (v18 task_type_contracts)
    IF NOT EXISTS (
      SELECT 1
      FROM task_type_contracts t
      WHERE t.project_id = p_project_id
        AND t.task_type = r.task_type
        AND t.pipeline_version = r.pipeline_version
        AND t.enabled = TRUE
    ) THEN
      UPDATE scheduled_runs
        SET status = 'error',
            reason_code = 'contract_missing',
            dispatched_at_utc = p_now_utc
      WHERE id = r.id;

      RETURN QUERY SELECT r.id, NULL::text, r.trace_id, 'error';
      CONTINUE;
    END IF;

    -- create run_id: keep text (uuid string)
    v_run_id := gen_random_uuid()::text;

    -- input_hash: stable hash. If input_template evidence exists, incorporate id; else 'nil'
    v_input_hash := encode(
      digest(
        p_project_id || '|' || r.task_type || '|' || r.pipeline_version || '|' || r.policy_version_id || '|' ||
        COALESCE(r.input_template_evidence_asset_id::text, 'nil') || '|' || COALESCE(r.mode, 'nil'),
        'sha256'
      ),
      'hex'
    );

    -- create run row (minimal fields; the rest filled by worker/usecase)
    INSERT INTO runs(
      id, project_id, task_type, status, trace_id,
      schedule_id, scheduled_run_id,
      pipeline_version, policy_version_id, mode,
      input_hash,
      created_at
    )
    VALUES (
      v_run_id, p_project_id, r.task_type, 'queued', r.trace_id,
      r.schedule_id, r.id,
      r.pipeline_version, r.policy_version_id, r.mode,
      v_input_hash,
      now()
    );

    UPDATE scheduled_runs
      SET status = 'dispatched',
          run_id = v_run_id,
          dispatched_at_utc = p_now_utc
    WHERE id = r.id;

    RETURN QUERY SELECT r.id, v_run_id, r.trace_id, 'dispatched';
  END LOOP;

  RETURN;
END;
$$;

-- 3) apply skip status (budget/policy gate results) as exec-only.
-- This is called by dispatcher after it evaluated budget/policy (default deny) and generated reason_evidence_asset_id.
CREATE OR REPLACE FUNCTION runsched_mark_skipped_v19(
  p_project_id VARCHAR,
  p_scheduled_run_id BIGINT,
  p_status TEXT,
  p_reason_code TEXT,
  p_reason_evidence_asset_id BIGINT
)
RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
  IF p_status NOT IN ('skipped_budget','skipped_policy') THEN
    RAISE EXCEPTION 'invalid status: %', p_status;
  END IF;

  UPDATE scheduled_runs
    SET status = p_status,
        reason_code = p_reason_code,
        reason_evidence_asset_id = p_reason_evidence_asset_id,
        dispatched_at_utc = now()
  WHERE project_id = p_project_id
    AND id = p_scheduled_run_id
    AND status = 'queued';
END;
$$;

-- 4) record internal error (throw禁止: evidence参照のみ)
CREATE OR REPLACE FUNCTION runsched_mark_error_v19(
  p_project_id VARCHAR,
  p_scheduled_run_id BIGINT,
  p_reason_code TEXT,
  p_error_detail_evidence_asset_id BIGINT
)
RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
  UPDATE scheduled_runs
    SET status = 'error',
        reason_code = p_reason_code,
        error_detail_evidence_asset_id = p_error_detail_evidence_asset_id,
        dispatched_at_utc = now()
  WHERE project_id = p_project_id
    AND id = p_scheduled_run_id;
END;
$$;



CREATE OR REPLACE FUNCTION runsched_claim_queued_scheduled_runs_v19(
  p_project_id VARCHAR,
  p_limit INTEGER
)
RETURNS TABLE(
  scheduled_run_id BIGINT,
  schedule_id BIGINT,
  trace_id TEXT
)
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
  RETURN QUERY
    SELECT sr.id, sr.schedule_id, sr.trace_id
    FROM scheduled_runs sr
    WHERE sr.project_id = p_project_id
      AND sr.status = 'queued'
    ORDER BY sr.scheduled_for_utc ASC, sr.id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT p_limit;
END;
$$;

CREATE OR REPLACE FUNCTION runsched_create_run_for_scheduled_v19(
  p_project_id VARCHAR,
  p_scheduled_run_id BIGINT,
  p_now_utc TIMESTAMPTZ
)
RETURNS TABLE(
  scheduled_run_id BIGINT,
  run_id TEXT,
  trace_id TEXT,
  status TEXT
)
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
  r RECORD;
  v_run_id TEXT;
  v_input_hash TEXT;
BEGIN
  SELECT sr.*, rs.task_type, rs.pipeline_version, rs.policy_version_id, rs.mode,
         rs.input_template_evidence_asset_id
    INTO r
  FROM scheduled_runs sr
  JOIN run_schedules rs ON rs.id = sr.schedule_id
  WHERE sr.project_id = p_project_id
    AND sr.id = p_scheduled_run_id
    AND sr.status = 'queued'
  FOR UPDATE;

  IF NOT FOUND THEN
    RETURN QUERY SELECT p_scheduled_run_id, NULL::text, NULL::text, 'not_found_or_not_queued';
    RETURN;
  END IF;

  -- v18 task_type_contracts check (must exist & enabled)
  IF NOT EXISTS (
    SELECT 1
    FROM task_type_contracts t
    WHERE t.project_id = p_project_id
      AND t.task_type = r.task_type
      AND t.pipeline_version = r.pipeline_version
      AND t.enabled = TRUE
  ) THEN
    UPDATE scheduled_runs
      SET status = 'error',
          reason_code = 'contract_missing',
          dispatched_at_utc = p_now_utc
    WHERE id = r.id;

    RETURN QUERY SELECT r.id, NULL::text, r.trace_id, 'error';
    RETURN;
  END IF;

  v_run_id := gen_random_uuid()::text;

  v_input_hash := encode(
    digest(
      p_project_id || '|' || r.task_type || '|' || r.pipeline_version || '|' || r.policy_version_id || '|' ||
      COALESCE(r.input_template_evidence_asset_id::text, 'nil') || '|' || COALESCE(r.mode, 'nil'),
      'sha256'
    ),
    'hex'
  );

  INSERT INTO runs(
    id, project_id, task_type, status, trace_id,
    schedule_id, scheduled_run_id,
    pipeline_version, policy_version_id, mode,
    input_hash,
    created_at
  )
  VALUES (
    v_run_id, p_project_id, r.task_type, 'queued', r.trace_id,
    r.schedule_id, r.id,
    r.pipeline_version, r.policy_version_id, r.mode,
    v_input_hash,
    now()
  );

  UPDATE scheduled_runs
    SET status = 'dispatched',
        run_id = v_run_id,
        dispatched_at_utc = p_now_utc
  WHERE id = r.id;

  RETURN QUERY SELECT r.id, v_run_id, r.trace_id, 'dispatched';
END;
$$;

COMMIT;