-- 0109_v143_rebuild_exec_only_grants.sql
BEGIN;

GRANT EXECUTE ON FUNCTION public.ledger_rebuild_run_accept_v14(
  text,text,timestamptz,timestamptz,text,text,text,text,jsonb
) TO ak;

GRANT EXECUTE ON FUNCTION public.ledger_rebuild_run_mark_running_v14(uuid) TO ak;
GRANT EXECUTE ON FUNCTION public.ledger_rebuild_run_mark_succeeded_v14(uuid,jsonb,jsonb) TO ak;
GRANT EXECUTE ON FUNCTION public.ledger_rebuild_run_mark_failed_recorded_v14(uuid,jsonb,jsonb) TO ak;
GRANT EXECUTE ON FUNCTION public.ledger_rebuild_run_dry_run_compute_v14(text,timestamptz,timestamptz) TO ak;

COMMIT;