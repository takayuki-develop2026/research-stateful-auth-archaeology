-- 0115_v143_rebuild_apply_exec_only_grants.sql
BEGIN;

GRANT EXECUTE ON FUNCTION public.ledger_rebuild_apply_v14(
  text,timestamptz,timestamptz,boolean,text,text,text,text,text,jsonb
) TO ak;

COMMIT;