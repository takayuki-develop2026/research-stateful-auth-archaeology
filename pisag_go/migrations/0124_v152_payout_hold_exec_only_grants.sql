-- 0124_v152_payout_hold_exec_only_grants.sql
BEGIN;

GRANT EXECUTE ON FUNCTION public.hold_consume_v152(varchar,text,boolean,text,text,text,jsonb) TO ak;
GRANT EXECUTE ON FUNCTION public.payout_schedule_with_hold_v152(varchar,text,text,text,text,bigint,char(64),date,text,timestamptz,text,boolean,text,text,text,text,jsonb) TO ak;
GRANT EXECUTE ON FUNCTION public.payout_mark_completed_consume_hold_from_utl_v152(varchar,text,varchar(128),boolean,text,text,text,jsonb) TO ak;

COMMIT;