-- 0090_v23_decisioncore_role_exec_only.sql
-- v23 DecisionCore: enforce EXECUTE-ONLY DB role (no direct table writes)
-- Target: decisioncore_server / decisioncore_worker will connect as role "decisioncoresvc"
--
-- NOTE (dev/local):
--   This file sets a default password. For production, rotate immediately and store in secrets.
--   Example:
--     ALTER ROLE decisioncoresvc PASSWORD '<strong-random>';
--
-- Requirements:
--  - v23 functions exist in public (policy_evaluation_upsert_v23, decision_*_v23, etc.)
--  - v18 evidence_register_v18 exists in public and is SECURITY DEFINER

BEGIN;

-- 1) Create role if missing
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'decisioncoresvc') THEN
    CREATE ROLE decisioncoresvc LOGIN PASSWORD 'decisioncoresvc';
  END IF;
END $$;

-- 2) Ensure the role can connect to this database
DO $$
DECLARE
  dbname text := current_database();
BEGIN
  EXECUTE format('GRANT CONNECT ON DATABASE %I TO decisioncoresvc', dbname);
END $$;

-- 3) Schema usage (needed to execute functions in public)
GRANT USAGE ON SCHEMA public TO decisioncoresvc;

-- 4) Hard deny: no direct table/sequence privileges
REVOKE ALL ON ALL TABLES    IN SCHEMA public FROM decisioncoresvc;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM decisioncoresvc;

-- (Optional) Also revoke any default privileges that might grant future tables to this role.
-- This is defensive; if you manage default privileges elsewhere, keep as-is.
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON TABLES    FROM decisioncoresvc;
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON SEQUENCES FROM decisioncoresvc;
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON FUNCTIONS FROM decisioncoresvc;

-- 5) Grant EXECUTE on required functions.
-- We intentionally grant by function name (all overloads) to avoid signature drift.
DO $$
DECLARE
  fn text;
BEGIN
  -- ---- v23 DecisionCore functions (ALL overloads) ----
  FOR fn IN
    SELECT p.oid::regprocedure::text
    FROM pg_proc p
    JOIN pg_namespace n ON n.oid = p.pronamespace
    WHERE n.nspname = 'public'
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
    EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO decisioncoresvc', fn);
  END LOOP;

  -- ---- v18 evidence register (exec-only) ----
  -- confirmed signature:
  -- evidence_register_v18(
  --   p_project_id varchar, p_trace_id varchar, p_actor_type varchar, p_actor_id varchar,
  --   p_media_type varchar, p_mime_type varchar, p_source_kind varchar, p_source_uri text,
  --   p_content_sha256 text, p_content_length bigint, p_language varchar, p_retention_policy varchar,
  --   p_expires_at_utc timestamptz, p_idempotency_key text
  -- ) RETURNS TABLE(evidence_ref uuid, found_existing boolean)
  FOR fn IN
    SELECT p.oid::regprocedure::text
    FROM pg_proc p
    JOIN pg_namespace n ON n.oid = p.pronamespace
    WHERE n.nspname = 'public'
      AND p.proname = 'evidence_register_v18'
  LOOP
    EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO decisioncoresvc', fn);
  END LOOP;

END $$;

COMMIT;