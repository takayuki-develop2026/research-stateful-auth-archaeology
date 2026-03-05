-- 0101_v14_ledger_ingest_grants.sql
BEGIN;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ak_worker_exec_only') THEN
    GRANT EXECUTE ON FUNCTION ledger_ingest_run_accept_v14(
      text, ledger_ingest_mode_v14, text, timestamptz, timestamptz, jsonb, text, text, text, text, jsonb
    ) TO ak_worker_exec_only;

    GRANT EXECUTE ON FUNCTION ledger_ingest_run_claim_next_v14(text) TO ak_worker_exec_only;
    GRANT EXECUTE ON FUNCTION ledger_ingest_run_touch_v14(uuid) TO ak_worker_exec_only;
    GRANT EXECUTE ON FUNCTION ledger_ingest_run_mark_succeeded_v14(uuid, jsonb, jsonb) TO ak_worker_exec_only;
    GRANT EXECUTE ON FUNCTION ledger_ingest_run_mark_failed_recorded_v14(uuid, jsonb, jsonb) TO ak_worker_exec_only;
  END IF;
END$$;

COMMIT;