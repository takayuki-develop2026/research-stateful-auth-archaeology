BEGIN;

GRANT EXECUTE ON FUNCTION public.refund_post_to_ledger_v151(varchar,text,boolean,text,text,text,jsonb) TO ak;
GRANT EXECUTE ON FUNCTION public.payout_post_to_ledger_v151(varchar,text,boolean,text,text,text,jsonb) TO ak;

COMMIT;