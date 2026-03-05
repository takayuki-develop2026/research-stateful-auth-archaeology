-- 0112_v144_period_close_exec_only_grants.sql
BEGIN;

GRANT EXECUTE ON FUNCTION public.ledger_period_close_v14(
  text,text,text,boolean,text,text,text,jsonb
) TO ak;

COMMIT;