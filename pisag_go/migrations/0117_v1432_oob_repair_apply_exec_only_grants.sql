-- 0117_v1432_oob_repair_apply_exec_only_grants.sql
BEGIN;

GRANT EXECUTE ON FUNCTION public.ledger_rebuild_apply_oob_repair_v1432(
  text,timestamptz,timestamptz,boolean,text,text,text,text,text,text,jsonb
) TO ak;

COMMIT;