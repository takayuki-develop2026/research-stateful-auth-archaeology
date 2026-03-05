-- 0106_v142_balance_exec_only_grants.sql
BEGIN;

-- Allow ak role (same as your other exec-only patterns)
GRANT EXECUTE ON FUNCTION public.ledger_balance_snapshot_upsert_day_v14(
  text, date, text, text, text, jsonb
) TO ak;

-- helpers are internal; not required but safe to keep exec-only for ak if needed
GRANT EXECUTE ON FUNCTION public._jsonb_is_array_v14(jsonb) TO ak;
GRANT EXECUTE ON FUNCTION public.sha256_hex_v14(text) TO ak;

COMMIT;