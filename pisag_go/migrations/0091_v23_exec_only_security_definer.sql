-- 0091_v23_exec_only_security_definer.sql
-- Make v23 write-functions SECURITY DEFINER so exec-only role can operate without table privileges.
-- Applies to: policy_evaluation_upsert_v23, decision_propose_v23, decision_approve_v23, decision_deny_v23,
--            decision_supersede_v23, decision_action_enqueue_v23, decision_action_claim_next_v23,
--            decision_action_mark_*_v23

BEGIN;

-- Helper: ensure functions run in public schema, not caller-controlled search_path.
-- IMPORTANT: execute as DB owner (ak).

-- 1) policy_evaluation_upsert_v23
ALTER FUNCTION public.policy_evaluation_upsert_v23(
  TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,NUMERIC,BIGINT,BIGINT,BIGINT,BIGINT
) SECURITY DEFINER;
ALTER FUNCTION public.policy_evaluation_upsert_v23(
  TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,NUMERIC,BIGINT,BIGINT,BIGINT,BIGINT
) SET search_path TO public;

-- 2) decision_propose_v23
ALTER FUNCTION public.decision_propose_v23(
  TEXT,TEXT,UUID,
  TEXT,TEXT,TEXT,
  TEXT,TEXT,
  TEXT,TEXT,TEXT,
  BIGINT,BIGINT,BIGINT,
  BIGINT,
  TEXT,TEXT,
  TEXT,TIMESTAMPTZ
) SECURITY DEFINER;
ALTER FUNCTION public.decision_propose_v23(
  TEXT,TEXT,UUID,
  TEXT,TEXT,TEXT,
  TEXT,TEXT,
  TEXT,TEXT,TEXT,
  BIGINT,BIGINT,BIGINT,
  BIGINT,
  TEXT,TEXT,
  TEXT,TIMESTAMPTZ
) SET search_path TO public;

-- 3) decision_approve_v23
ALTER FUNCTION public.decision_approve_v23(BIGINT,TEXT,TEXT,TEXT,BIGINT) SECURITY DEFINER;
ALTER FUNCTION public.decision_approve_v23(BIGINT,TEXT,TEXT,TEXT,BIGINT) SET search_path TO public;

-- 4) decision_deny_v23
ALTER FUNCTION public.decision_deny_v23(BIGINT,TEXT,TEXT,TEXT,BIGINT) SECURITY DEFINER;
ALTER FUNCTION public.decision_deny_v23(BIGINT,TEXT,TEXT,TEXT,BIGINT) SET search_path TO public;

-- 5) decision_supersede_v23
ALTER FUNCTION public.decision_supersede_v23(BIGINT,BIGINT,TEXT,BIGINT) SECURITY DEFINER;
ALTER FUNCTION public.decision_supersede_v23(BIGINT,BIGINT,TEXT,BIGINT) SET search_path TO public;

-- 6) decision_action_enqueue_v23
ALTER FUNCTION public.decision_action_enqueue_v23(
  TEXT,TEXT,UUID,BIGINT,
  TEXT,TEXT,TEXT,
  TEXT,BIGINT,BIGINT,
  TEXT,BIGINT,
  TEXT,BIGINT
) SECURITY DEFINER;
ALTER FUNCTION public.decision_action_enqueue_v23(
  TEXT,TEXT,UUID,BIGINT,
  TEXT,TEXT,TEXT,
  TEXT,BIGINT,BIGINT,
  TEXT,BIGINT,
  TEXT,BIGINT
) SET search_path TO public;

-- 7) decision_action_claim_next_v23
ALTER FUNCTION public.decision_action_claim_next_v23(TEXT) SECURITY DEFINER;
ALTER FUNCTION public.decision_action_claim_next_v23(TEXT) SET search_path TO public;

-- 8) mark_* functions
ALTER FUNCTION public.decision_action_mark_succeeded_v23(BIGINT,TEXT) SECURITY DEFINER;
ALTER FUNCTION public.decision_action_mark_succeeded_v23(BIGINT,TEXT) SET search_path TO public;

ALTER FUNCTION public.decision_action_mark_failed_soft_v23(BIGINT,TEXT,BIGINT) SECURITY DEFINER;
ALTER FUNCTION public.decision_action_mark_failed_soft_v23(BIGINT,TEXT,BIGINT) SET search_path TO public;

ALTER FUNCTION public.decision_action_mark_skipped_budget_v23(BIGINT,TEXT,BIGINT) SECURITY DEFINER;
ALTER FUNCTION public.decision_action_mark_skipped_budget_v23(BIGINT,TEXT,BIGINT) SET search_path TO public;

ALTER FUNCTION public.decision_action_mark_blocked_policy_v23(BIGINT,TEXT,BIGINT) SECURITY DEFINER;
ALTER FUNCTION public.decision_action_mark_blocked_policy_v23(BIGINT,TEXT,BIGINT) SET search_path TO public;

ALTER FUNCTION public.decision_action_mark_review_required_v23(BIGINT,TEXT,BIGINT) SECURITY DEFINER;
ALTER FUNCTION public.decision_action_mark_review_required_v23(BIGINT,TEXT,BIGINT) SET search_path TO public;

COMMIT;