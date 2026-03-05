-- 0120_v15_funds_exec_only_grants.sql
BEGIN;

-- grants to ak role (same pattern as v14)
GRANT EXECUTE ON FUNCTION public.funds_op_open_v15(varchar,text,text,text,text,text,text,text,text,jsonb) TO ak;
GRANT EXECUTE ON FUNCTION public.funds_op_resolve_v15(uuid,text,text,text,text,jsonb) TO ak;

GRANT EXECUTE ON FUNCTION public.refund_create_requested_v15(varchar,text,text,text,text,bigint,char(64),boolean,text,text,text,text,jsonb) TO ak;
GRANT EXECUTE ON FUNCTION public.refund_mark_succeeded_from_utl_v15(varchar,text,varchar(128),text,text,text,jsonb) TO ak;
GRANT EXECUTE ON FUNCTION public.refund_mark_failed_v15(varchar,text,text,text,boolean,text,text,text,jsonb) TO ak;

GRANT EXECUTE ON FUNCTION public.payout_schedule_v15(varchar,text,text,text,text,bigint,char(64),date,boolean,text,text,text,text,jsonb) TO ak;
GRANT EXECUTE ON FUNCTION public.payout_mark_completed_from_utl_v15(varchar,text,varchar(128),text,text,text,jsonb) TO ak;
GRANT EXECUTE ON FUNCTION public.payout_mark_failed_v15(varchar,text,text,text,boolean,timestamptz,text,text,text,jsonb) TO ak;

GRANT EXECUTE ON FUNCTION public.hold_create_v15(varchar,text,text,text,text,text,bigint,text,timestamptz,boolean,text,text,text,text,jsonb) TO ak;
GRANT EXECUTE ON FUNCTION public.hold_release_v15(varchar,text,boolean,text,text,text,jsonb) TO ak;

GRANT EXECUTE ON FUNCTION public.dispute_upsert_from_utl_v15(varchar,text,text,text,bigint,text,timestamptz,timestamptz,varchar(128),text,text,text,jsonb) TO ak;
GRANT EXECUTE ON FUNCTION public.dispute_event_insert_v15(varchar,text,varchar(128),text,timestamptz,uuid,text,text) TO ak;

GRANT EXECUTE ON FUNCTION public.settlement_batch_create_v15(varchar,text,text,timestamptz,timestamptz,text,boolean,text,text,text,text,jsonb) TO ak;
GRANT EXECUTE ON FUNCTION public.settlement_reconcile_dry_run_v15(varchar,text,jsonb,text,text,text,jsonb) TO ak;

COMMIT;