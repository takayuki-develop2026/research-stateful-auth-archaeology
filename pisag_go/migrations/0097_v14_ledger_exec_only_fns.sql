-- 0097_v14_ledger_exec_only_fns.sql
-- v14.0 EXECUTE ONLY functions:
-- - ledger_posting_create_v14: idempotent create by (project_id, posting_key)
-- - ledger_entries_insert_v14: insert lines with strict validation (no unknown account, currency match)
-- - ledger_posting_finalize_v14: DB-side double-entry enforcement (single currency + zero-sum)

BEGIN;

-- =========================================================
-- Helper: safe jsonb array check
-- =========================================================
CREATE OR REPLACE FUNCTION _jsonb_is_array(p jsonb)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
AS $$
  SELECT COALESCE(jsonb_typeof(p) = 'array', false);
$$;

-- =========================================================
-- 1) Create posting (idempotent)
-- =========================================================
CREATE OR REPLACE FUNCTION ledger_posting_create_v14(
  p_project_id text,
  p_posting_key text,
  p_source_event_key text,
  p_posting_type ledger_posting_type_v14,
  p_currency text,
  p_posted_at timestamptz,
  p_run_id text,
  p_trace_id text,
  p_policy_version_id text,
  p_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE (posting_id uuid, status text)
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
  IF p_posting_key IS NULL OR length(trim(p_posting_key)) = 0 THEN
    RAISE EXCEPTION 'posting_key is required';
  END IF;
  IF p_source_event_key IS NULL OR length(trim(p_source_event_key)) = 0 THEN
    RAISE EXCEPTION 'source_event_key is required';
  END IF;
  IF p_currency IS NULL OR length(trim(p_currency)) = 0 THEN
    RAISE EXCEPTION 'currency is required';
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
  IF NOT _jsonb_is_array(p_evidence_refs) THEN
    RAISE EXCEPTION 'evidence_refs must be a json array';
  END IF;

  -- Try insert; if conflict, return existing
  INSERT INTO ledger_postings(
    project_id, posting_key, source_event_key, posting_type, currency,
    status, posted_at, run_id, trace_id, policy_version_id, evidence_refs
  )
  VALUES (
    p_project_id, p_posting_key, p_source_event_key, p_posting_type, p_currency,
    'draft', p_posted_at, p_run_id, p_trace_id, p_policy_version_id, p_evidence_refs
  )
  ON CONFLICT (project_id, posting_key) DO NOTHING
  RETURNING id INTO v_id;

  IF v_id IS NOT NULL THEN
    posting_id := v_id;
    status := 'created';
    RETURN NEXT;
    RETURN;
  END IF;

  SELECT lp.id INTO v_id
  FROM ledger_postings lp
  WHERE lp.project_id = p_project_id AND lp.posting_key = p_posting_key;

  posting_id := v_id;
  status := 'already_exists';
  RETURN NEXT;
END;
$$;

-- =========================================================
-- 2) Insert entries for a posting (strict validation)
-- =========================================================
CREATE OR REPLACE FUNCTION ledger_entries_insert_v14(
  p_posting_id uuid,
  p_entries jsonb
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text;
  v_currency text;

  e jsonb;

  v_account_key text;
  v_account_id uuid;
  v_direction text;
  v_amount bigint;
  v_entry_currency text;
  v_entry_key text;
  v_evidence_refs jsonb;
BEGIN
  IF p_posting_id IS NULL THEN
    RAISE EXCEPTION 'posting_id is required';
  END IF;
  IF NOT _jsonb_is_array(p_entries) THEN
    RAISE EXCEPTION 'entries must be a json array';
  END IF;

  SELECT lp.project_id, lp.currency
    INTO v_project_id, v_currency
  FROM ledger_postings lp
  WHERE lp.id = p_posting_id;

  IF v_project_id IS NULL THEN
    RAISE EXCEPTION 'posting not found';
  END IF;

  FOR e IN SELECT * FROM jsonb_array_elements(p_entries)
  LOOP
    v_account_key := NULLIF(trim(COALESCE(e->>'account_key','')), '');
    v_direction   := NULLIF(trim(COALESCE(e->>'direction','')), '');
    v_entry_key   := NULLIF(trim(COALESCE(e->>'entry_key','')), '');
    v_entry_currency := NULLIF(trim(COALESCE(e->>'currency','')), '');
    v_amount := COALESCE((e->>'amount')::bigint, -1);

    IF v_account_key IS NULL THEN
      RAISE EXCEPTION 'entry.account_key is required';
    END IF;
    IF v_direction NOT IN ('debit','credit') THEN
      RAISE EXCEPTION 'entry.direction must be debit or credit';
    END IF;
    IF v_entry_key IS NULL THEN
      RAISE EXCEPTION 'entry.entry_key is required';
    END IF;
    IF v_entry_currency IS NULL THEN
      RAISE EXCEPTION 'entry.currency is required';
    END IF;
    IF v_entry_currency <> v_currency THEN
      RAISE EXCEPTION 'entry.currency must match posting currency';
    END IF;
    IF v_amount < 0 THEN
      RAISE EXCEPTION 'entry.amount must be >= 0';
    END IF;

    v_evidence_refs := COALESCE(e->'evidence_refs', '[]'::jsonb);
    IF NOT _jsonb_is_array(v_evidence_refs) THEN
      RAISE EXCEPTION 'entry.evidence_refs must be a json array';
    END IF;

    -- Resolve account strictly: do NOT auto-create
    SELECT la.id INTO v_account_id
    FROM ledger_accounts la
    WHERE la.project_id = v_project_id
      AND la.account_key = v_account_key
      AND la.currency = v_currency
      AND la.status = 'active';

    IF v_account_id IS NULL THEN
      RAISE EXCEPTION 'unknown_or_inactive_account: %', v_account_key;
    END IF;

    INSERT INTO ledger_entries(
      project_id, posting_id, account_id, direction, amount, currency, entry_key, evidence_refs
    )
    VALUES(
      v_project_id, p_posting_id, v_account_id, v_direction::ledger_direction_v14,
      v_amount, v_currency, v_entry_key, v_evidence_refs
    )
    ON CONFLICT (posting_id, entry_key) DO NOTHING;
  END LOOP;
END;
$$;

-- =========================================================
-- 3) Finalize posting (DB-side invariants)
-- FIX: qualify "status" column with alias to avoid ambiguity (42702)
-- =========================================================
CREATE OR REPLACE FUNCTION ledger_posting_finalize_v14(
  p_posting_id uuid,
  p_append_evidence_refs jsonb DEFAULT '[]'::jsonb
)
RETURNS TABLE (posting_id uuid, status ledger_posting_status_v14, debit_total bigint, credit_total bigint)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_project_id text;
  v_posting_currency text;
  v_posting_status ledger_posting_status_v14;

  v_debit bigint := 0;
  v_credit bigint := 0;
  v_distinct_currency_count int := 0;

  v_existing_refs jsonb;
  v_new_refs jsonb;
BEGIN
  IF p_posting_id IS NULL THEN
    RAISE EXCEPTION 'posting_id is required';
  END IF;
  IF NOT _jsonb_is_array(p_append_evidence_refs) THEN
    RAISE EXCEPTION 'append_evidence_refs must be a json array';
  END IF;

  SELECT lp.project_id, lp.currency, lp.status, lp.evidence_refs
    INTO v_project_id, v_posting_currency, v_posting_status, v_existing_refs
  FROM ledger_postings lp
  WHERE lp.id = p_posting_id
  FOR UPDATE;

  IF v_project_id IS NULL THEN
    RAISE EXCEPTION 'posting not found';
  END IF;

  -- If already posted/voided, return as-is (idempotent finalize)
  IF v_posting_status IN ('posted','voided') THEN
    posting_id := p_posting_id;
    status := v_posting_status;

    SELECT COALESCE(sum(le.amount),0) INTO v_debit
      FROM ledger_entries le WHERE le.posting_id = p_posting_id AND le.direction = 'debit';
    SELECT COALESCE(sum(le.amount),0) INTO v_credit
      FROM ledger_entries le WHERE le.posting_id = p_posting_id AND le.direction = 'credit';

    debit_total := v_debit;
    credit_total := v_credit;
    RETURN NEXT;
    RETURN;
  END IF;

  -- currency mix check (should be 1)
  SELECT COUNT(DISTINCT le.currency) INTO v_distinct_currency_count
  FROM ledger_entries le
  WHERE le.posting_id = p_posting_id;

  IF v_distinct_currency_count > 1 THEN
    v_new_refs := COALESCE(v_existing_refs, '[]'::jsonb) || p_append_evidence_refs;

    UPDATE ledger_postings lp
      SET status = 'failed_recorded',
          updated_at = now(),
          evidence_refs = v_new_refs
    WHERE lp.id = p_posting_id;

    posting_id := p_posting_id;
    status := 'failed_recorded';
    debit_total := 0;
    credit_total := 0;
    RETURN NEXT;
    RETURN;
  END IF;

  SELECT COALESCE(sum(le.amount),0) INTO v_debit
  FROM ledger_entries le
  WHERE le.posting_id = p_posting_id AND le.direction = 'debit';

  SELECT COALESCE(sum(le.amount),0) INTO v_credit
  FROM ledger_entries le
  WHERE le.posting_id = p_posting_id AND le.direction = 'credit';

  IF v_debit <> v_credit THEN
    v_new_refs := COALESCE(v_existing_refs, '[]'::jsonb) || p_append_evidence_refs;

    UPDATE ledger_postings lp
      SET status = 'failed_recorded',
          updated_at = now(),
          evidence_refs = v_new_refs
    WHERE lp.id = p_posting_id;

    posting_id := p_posting_id;
    status := 'failed_recorded';
    debit_total := v_debit;
    credit_total := v_credit;
    RETURN NEXT;
    RETURN;
  END IF;

  -- OK -> posted
  v_new_refs := COALESCE(v_existing_refs, '[]'::jsonb) || p_append_evidence_refs;

  UPDATE ledger_postings lp
    SET status = 'posted',
        updated_at = now(),
        evidence_refs = v_new_refs
  WHERE lp.id = p_posting_id;

  posting_id := p_posting_id;
  status := 'posted';
  debit_total := v_debit;
  credit_total := v_credit;
  RETURN NEXT;
END;
$$;

-- =========================================================
-- SECURITY: revoke from PUBLIC (fail-closed)
-- =========================================================
REVOKE ALL ON FUNCTION ledger_posting_create_v14(
  text, text, text, ledger_posting_type_v14, text, timestamptz, text, text, text, jsonb
) FROM PUBLIC;

REVOKE ALL ON FUNCTION ledger_entries_insert_v14(uuid, jsonb) FROM PUBLIC;

REVOKE ALL ON FUNCTION ledger_posting_finalize_v14(uuid, jsonb) FROM PUBLIC;

REVOKE ALL ON FUNCTION _jsonb_is_array(jsonb) FROM PUBLIC;

COMMIT;