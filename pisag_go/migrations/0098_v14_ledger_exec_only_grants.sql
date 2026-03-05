-- 0098_v14_ledger_exec_only_grants.sql
-- Grant EXECUTE to exec-only role(s) if they exist. No-op if absent.

BEGIN;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ak_worker_exec_only') THEN
    GRANT EXECUTE ON FUNCTION ledger_posting_create_v14(
      text, text, text, ledger_posting_type_v14, text, timestamptz, text, text, text, jsonb
    ) TO ak_worker_exec_only;

    GRANT EXECUTE ON FUNCTION ledger_entries_insert_v14(uuid, jsonb)
    TO ak_worker_exec_only;

    GRANT EXECUTE ON FUNCTION ledger_posting_finalize_v14(uuid, jsonb)
    TO ak_worker_exec_only;
  END IF;

  -- Add more roles if you already use them (optional)
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ak_admin') THEN
    GRANT EXECUTE ON FUNCTION ledger_posting_create_v14(
      text, text, text, ledger_posting_type_v14, text, timestamptz, text, text, text, jsonb
    ) TO ak_admin;

    GRANT EXECUTE ON FUNCTION ledger_entries_insert_v14(uuid, jsonb)
    TO ak_admin;

    GRANT EXECUTE ON FUNCTION ledger_posting_finalize_v14(uuid, jsonb)
    TO ak_admin;
  END IF;
END$$;

COMMIT;