-- 0089_v23_decisioncore_exec_only.sql (SAFE VERSION)
BEGIN;

CREATE OR REPLACE FUNCTION _now_utc_v23()
RETURNS TIMESTAMPTZ
LANGUAGE sql
AS $$ SELECT now(); $$;

-- 1) Policy Evaluation Upsert
CREATE OR REPLACE FUNCTION policy_evaluation_upsert_v23(
  p_project_id TEXT,
  p_trace_id TEXT,
  p_run_id UUID,
  p_policy_version_str TEXT,
  p_pipeline_version TEXT,
  p_input_hash TEXT,
  p_pdp_mode TEXT,
  p_result TEXT,
  p_score NUMERIC,
  p_reason_asset_id BIGINT,
  p_obligations_asset_id BIGINT,
  p_proposal_asset_id BIGINT,
  p_policy_decision_id BIGINT
)
RETURNS BIGINT
LANGUAGE plpgsql
AS $$
DECLARE v_id BIGINT;
BEGIN
  INSERT INTO policy_evaluations_v23(
    project_id, trace_id, run_id,
    policy_version_str, pipeline_version,
    input_hash,
    pdp_mode, result, score,
    reason_evidence_asset_id, obligations_evidence_asset_id, proposal_evidence_asset_id,
    policy_decision_id,
    created_at_utc
  )
  VALUES (
    p_project_id, p_trace_id, p_run_id,
    p_policy_version_str, p_pipeline_version,
    p_input_hash,
    p_pdp_mode, p_result, p_score,
    p_reason_asset_id, p_obligations_asset_id, NULLIF(p_proposal_asset_id, 0),
    NULLIF(p_policy_decision_id, 0),
    _now_utc_v23()
  )
  ON CONFLICT (project_id, trace_id, input_hash, policy_version_str, pipeline_version)
  DO UPDATE SET
    run_id = EXCLUDED.run_id,
    pdp_mode = EXCLUDED.pdp_mode,
    result = EXCLUDED.result,
    score = EXCLUDED.score,
    reason_evidence_asset_id = EXCLUDED.reason_evidence_asset_id,
    obligations_evidence_asset_id = EXCLUDED.obligations_evidence_asset_id,
    proposal_evidence_asset_id = EXCLUDED.proposal_evidence_asset_id,
    policy_decision_id = EXCLUDED.policy_decision_id
  RETURNING id INTO v_id;

  RETURN v_id;
END;
$$;

-- 2) Decision Propose
CREATE OR REPLACE FUNCTION decision_propose_v23(
  p_project_id TEXT,
  p_trace_id TEXT,
  p_run_id UUID,

  p_subject_type TEXT,
  p_subject_id TEXT,
  p_subject_owner_project_id TEXT,

  p_decision_key TEXT,
  p_decision_scope TEXT,

  p_policy_version_str TEXT,
  p_pipeline_version TEXT,
  p_input_hash TEXT,

  p_inputs_asset_id BIGINT,
  p_proposal_asset_id BIGINT,
  p_obligations_asset_id BIGINT,

  p_policy_evaluation_id BIGINT,

  p_decided_by_type TEXT,
  p_decided_by_id TEXT,

  p_initial_status TEXT,
  p_expires_at_utc TIMESTAMPTZ
)
RETURNS BIGINT
LANGUAGE plpgsql
AS $$
DECLARE v_id BIGINT;
BEGIN
  INSERT INTO decision_ledgers_v23(
    project_id, trace_id, run_id,
    subject_type, subject_id, subject_owner_project_id,
    decision_key, decision_kind, decision_scope,
    policy_version_str, pipeline_version,
    input_hash,
    inputs_evidence_asset_id, proposal_evidence_asset_id, obligations_evidence_asset_id,
    policy_evaluation_id,
    decided_by_type, decided_by_id, decided_at_utc,
    status, expires_at_utc
  )
  VALUES (
    p_project_id, p_trace_id, p_run_id,
    p_subject_type, p_subject_id, p_subject_owner_project_id,
    p_decision_key, 'propose', p_decision_scope,
    p_policy_version_str, p_pipeline_version,
    p_input_hash,
    p_inputs_asset_id, NULLIF(p_proposal_asset_id, 0), p_obligations_asset_id,
    NULLIF(p_policy_evaluation_id, 0),
    p_decided_by_type, p_decided_by_id, _now_utc_v23(),
    p_initial_status, p_expires_at_utc
  )
  ON CONFLICT (project_id, decision_key)
  DO UPDATE SET
    trace_id = EXCLUDED.trace_id,
    run_id = EXCLUDED.run_id,
    policy_version_str = EXCLUDED.policy_version_str,
    pipeline_version = EXCLUDED.pipeline_version,
    input_hash = EXCLUDED.input_hash,
    inputs_evidence_asset_id = EXCLUDED.inputs_evidence_asset_id,
    proposal_evidence_asset_id = EXCLUDED.proposal_evidence_asset_id,
    obligations_evidence_asset_id = EXCLUDED.obligations_evidence_asset_id,
    policy_evaluation_id = EXCLUDED.policy_evaluation_id
  RETURNING id INTO v_id;

  RETURN v_id;
END;
$$;

-- 3) Approve / Deny
CREATE OR REPLACE FUNCTION decision_approve_v23(
  p_decision_id BIGINT,
  p_project_id TEXT,
  p_decided_by_type TEXT,
  p_decided_by_id TEXT,
  p_comment_asset_id BIGINT
)
RETURNS TABLE(decision_id BIGINT, new_status TEXT)
LANGUAGE plpgsql
AS $$
BEGIN
  UPDATE decision_ledgers_v23
  SET
    decision_kind = 'approve',
    decided_by_type = p_decided_by_type,
    decided_by_id = p_decided_by_id,
    decided_at_utc = _now_utc_v23(),
    status = 'approved',
    comment_evidence_asset_id = NULLIF(p_comment_asset_id, 0)
  WHERE id = p_decision_id
    AND project_id = p_project_id
    AND status IN ('proposed', 'review_required')
  RETURNING id, status INTO decision_id, new_status;

  IF decision_id IS NULL THEN
    RETURN QUERY
      SELECT d.id, d.status FROM decision_ledgers_v23 d
      WHERE d.id = p_decision_id AND d.project_id = p_project_id;
    RETURN;
  END IF;

  RETURN;
END;
$$;

CREATE OR REPLACE FUNCTION decision_deny_v23(
  p_decision_id BIGINT,
  p_project_id TEXT,
  p_decided_by_type TEXT,
  p_decided_by_id TEXT,
  p_comment_asset_id BIGINT
)
RETURNS TABLE(decision_id BIGINT, new_status TEXT)
LANGUAGE plpgsql
AS $$
BEGIN
  UPDATE decision_ledgers_v23
  SET
    decision_kind = 'deny',
    decided_by_type = p_decided_by_type,
    decided_by_id = p_decided_by_id,
    decided_at_utc = _now_utc_v23(),
    status = 'denied',
    comment_evidence_asset_id = NULLIF(p_comment_asset_id, 0)
  WHERE id = p_decision_id
    AND project_id = p_project_id
    AND status IN ('proposed', 'review_required')
  RETURNING id, status INTO decision_id, new_status;

  IF decision_id IS NULL THEN
    RETURN QUERY
      SELECT d.id, d.status FROM decision_ledgers_v23 d
      WHERE d.id = p_decision_id AND d.project_id = p_project_id;
    RETURN;
  END IF;

  RETURN;
END;
$$;

-- 4) Supersede
CREATE OR REPLACE FUNCTION decision_supersede_v23(
  p_old_decision_id BIGINT,
  p_new_decision_id BIGINT,
  p_project_id TEXT,
  p_comment_asset_id BIGINT
)
RETURNS VOID
LANGUAGE plpgsql
AS $$
BEGIN
  UPDATE decision_ledgers_v23
  SET
    status = 'superseded',
    superseded_by_decision_id = p_new_decision_id,
    comment_evidence_asset_id = COALESCE(NULLIF(p_comment_asset_id, 0), comment_evidence_asset_id)
  WHERE id = p_old_decision_id
    AND project_id = p_project_id
    AND status <> 'superseded';
END;
$$;

-- 5) Action Enqueue + Worker claim/mark
CREATE OR REPLACE FUNCTION decision_action_enqueue_v23(
  p_project_id TEXT,
  p_trace_id TEXT,
  p_run_id UUID,
  p_decision_ledger_id BIGINT,

  p_action_key TEXT,
  p_action_type TEXT,
  p_action_scope TEXT,

  p_target_hash TEXT,
  p_target_asset_id BIGINT,
  p_plan_asset_id BIGINT,

  p_budget_currency TEXT,
  p_budget_estimate_amount BIGINT,

  p_initial_status TEXT,
  p_error_asset_id BIGINT
)
RETURNS BIGINT
LANGUAGE plpgsql
AS $$
DECLARE v_id BIGINT;
BEGIN
  INSERT INTO decision_actions_v23(
    project_id, trace_id, run_id,
    decision_ledger_id,
    action_key,
    action_type, action_scope,
    target_hash, target_evidence_asset_id, plan_evidence_asset_id,
    budget_currency, budget_estimate_amount,
    status,
    error_evidence_asset_id
  )
  VALUES (
    p_project_id, p_trace_id, p_run_id,
    p_decision_ledger_id,
    p_action_key,
    p_action_type, p_action_scope,
    p_target_hash, p_target_asset_id, p_plan_asset_id,
    p_budget_currency, p_budget_estimate_amount,
    p_initial_status,
    NULLIF(p_error_asset_id, 0)
  )
  ON CONFLICT (project_id, action_key)
  DO UPDATE SET
    trace_id = EXCLUDED.trace_id,
    run_id = EXCLUDED.run_id,
    decision_ledger_id = EXCLUDED.decision_ledger_id,
    plan_evidence_asset_id = EXCLUDED.plan_evidence_asset_id,
    target_evidence_asset_id = EXCLUDED.target_evidence_asset_id,
    target_hash = EXCLUDED.target_hash,
    budget_currency = EXCLUDED.budget_currency,
    budget_estimate_amount = EXCLUDED.budget_estimate_amount,
    status = CASE
      WHEN decision_actions_v23.status = 'succeeded' THEN decision_actions_v23.status
      ELSE EXCLUDED.status
    END,
    error_evidence_asset_id = COALESCE(EXCLUDED.error_evidence_asset_id, decision_actions_v23.error_evidence_asset_id)
  RETURNING id INTO v_id;

  RETURN v_id;
END;
$$;

CREATE OR REPLACE FUNCTION decision_action_claim_next_v23(p_project_id TEXT)
RETURNS TABLE(
  action_id BIGINT,
  action_key TEXT,
  action_type TEXT,
  action_scope TEXT,
  decision_ledger_id BIGINT,
  trace_id TEXT,
  run_id UUID,
  target_hash TEXT,
  target_evidence_asset_id BIGINT,
  plan_evidence_asset_id BIGINT,
  budget_currency TEXT,
  budget_estimate_amount BIGINT
)
LANGUAGE plpgsql
AS $$
BEGIN
  RETURN QUERY
  WITH picked AS (
    SELECT id
    FROM decision_actions_v23
    WHERE project_id = p_project_id
      AND status = 'queued'
    ORDER BY id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
  ),
  upd AS (
    UPDATE decision_actions_v23 a
    SET status = 'running', started_at_utc = _now_utc_v23()
    WHERE a.id = (SELECT id FROM picked)
    RETURNING a.*
  )
  SELECT
    u.id, u.action_key, u.action_type, u.action_scope, u.decision_ledger_id,
    u.trace_id, u.run_id,
    u.target_hash, u.target_evidence_asset_id, u.plan_evidence_asset_id,
    u.budget_currency, u.budget_estimate_amount
  FROM upd u;
END;
$$;

CREATE OR REPLACE FUNCTION decision_action_mark_succeeded_v23(p_action_id BIGINT, p_project_id TEXT)
RETURNS VOID LANGUAGE plpgsql AS $$
BEGIN
  UPDATE decision_actions_v23
  SET status='succeeded', finished_at_utc=_now_utc_v23()
  WHERE id=p_action_id AND project_id=p_project_id;
END; $$;

CREATE OR REPLACE FUNCTION decision_action_mark_failed_soft_v23(p_action_id BIGINT, p_project_id TEXT, p_error_asset_id BIGINT)
RETURNS VOID LANGUAGE plpgsql AS $$
BEGIN
  UPDATE decision_actions_v23
  SET status='failed_soft', finished_at_utc=_now_utc_v23(), error_evidence_asset_id=NULLIF(p_error_asset_id,0)
  WHERE id=p_action_id AND project_id=p_project_id;
END; $$;

CREATE OR REPLACE FUNCTION decision_action_mark_skipped_budget_v23(p_action_id BIGINT, p_project_id TEXT, p_reason_asset_id BIGINT)
RETURNS VOID LANGUAGE plpgsql AS $$
BEGIN
  UPDATE decision_actions_v23
  SET status='skipped_budget', finished_at_utc=_now_utc_v23(), error_evidence_asset_id=NULLIF(p_reason_asset_id,0)
  WHERE id=p_action_id AND project_id=p_project_id;
END; $$;

CREATE OR REPLACE FUNCTION decision_action_mark_blocked_policy_v23(p_action_id BIGINT, p_project_id TEXT, p_reason_asset_id BIGINT)
RETURNS VOID LANGUAGE plpgsql AS $$
BEGIN
  UPDATE decision_actions_v23
  SET status='blocked_policy', finished_at_utc=_now_utc_v23(), error_evidence_asset_id=NULLIF(p_reason_asset_id,0)
  WHERE id=p_action_id AND project_id=p_project_id;
END; $$;

CREATE OR REPLACE FUNCTION decision_action_mark_review_required_v23(p_action_id BIGINT, p_project_id TEXT, p_reason_asset_id BIGINT)
RETURNS VOID LANGUAGE plpgsql AS $$
BEGIN
  UPDATE decision_actions_v23
  SET status='review_required', finished_at_utc=_now_utc_v23(), error_evidence_asset_id=NULLIF(p_reason_asset_id,0)
  WHERE id=p_action_id AND project_id=p_project_id;
END; $$;

-- -----------------------------
-- EXECUTE ONLY permissions (SAFE):
-- revoke/grant by function name (all overloads), not by signature.
-- -----------------------------
DO $$
DECLARE
  r TEXT;
  roles TEXT[] := ARRAY['ak_worker','ak_admin_api','govsvc','opssvc','agentsvc','postgres'];
  fn TEXT;
BEGIN
  FOR fn IN
    SELECT p.oid::regprocedure::text
    FROM pg_proc p
    JOIN pg_namespace n ON n.oid=p.pronamespace
    WHERE n.nspname='public'
      AND p.proname IN (
        '_now_utc_v23',
        'policy_evaluation_upsert_v23',
        'decision_propose_v23',
        'decision_approve_v23',
        'decision_deny_v23',
        'decision_supersede_v23',
        'decision_action_enqueue_v23',
        'decision_action_claim_next_v23',
        'decision_action_mark_succeeded_v23',
        'decision_action_mark_failed_soft_v23',
        'decision_action_mark_skipped_budget_v23',
        'decision_action_mark_blocked_policy_v23',
        'decision_action_mark_review_required_v23'
      )
  LOOP
    EXECUTE format('REVOKE ALL ON FUNCTION %s FROM PUBLIC', fn);
  END LOOP;

  FOREACH r IN ARRAY roles LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
      FOR fn IN
        SELECT p.oid::regprocedure::text
        FROM pg_proc p
        JOIN pg_namespace n ON n.oid=p.pronamespace
        WHERE n.nspname='public'
          AND p.proname IN (
            'policy_evaluation_upsert_v23',
            'decision_propose_v23',
            'decision_approve_v23',
            'decision_deny_v23',
            'decision_supersede_v23',
            'decision_action_enqueue_v23',
            'decision_action_claim_next_v23',
            'decision_action_mark_succeeded_v23',
            'decision_action_mark_failed_soft_v23',
            'decision_action_mark_skipped_budget_v23',
            'decision_action_mark_blocked_policy_v23',
            'decision_action_mark_review_required_v23'
          )
      LOOP
        EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO %I', fn, r);
      END LOOP;
    END IF;
  END LOOP;
END $$;

COMMIT;