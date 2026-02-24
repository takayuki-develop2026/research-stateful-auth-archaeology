-- migrations/0000_roles_v41.sql (revised)
-- v4.1+ final runtime sandbox:
-- - ak_worker: NO direct SELECT/UPDATE on tables
-- - write-only via INSERT, and state transitions via SECURITY DEFINER functions
-- Run this as DB owner (ak) / superuser.

BEGIN;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ak_worker') THEN
    CREATE ROLE ak_worker LOGIN PASSWORD 'ak_worker';
  END IF;
END$$;

-- schema usage
GRANT USAGE ON SCHEMA public TO ak_worker;

-- “全部禁止”を明示（保険）
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM ak_worker;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM ak_worker;

-- =========================
-- Table privileges (write-only)
-- =========================
-- runs: INSERT only（UPDATEは関数経由に寄せる）
GRANT INSERT ON TABLE public.runs TO ak_worker;

-- run_inputs: INSERT only（claimは SECURITY DEFINER 関数で行う）
GRANT INSERT ON TABLE public.run_inputs TO ak_worker;

-- run_events: INSERT only
GRANT INSERT ON TABLE public.run_events TO ak_worker;

-- evidence assets: INSERT only（workerが保存するなら）
GRANT INSERT ON TABLE public.run_evidence_assets TO ak_worker;

-- =========================
-- Sequences (needed for bigserial)
-- =========================
GRANT USAGE, SELECT ON SEQUENCE public.run_inputs_id_seq TO ak_worker;
GRANT USAGE, SELECT ON SEQUENCE public.run_events_id_seq TO ak_worker;
GRANT USAGE, SELECT ON SEQUENCE public.run_evidence_assets_id_seq TO ak_worker;

-- =========================
-- Explicitly deny reads (intent)
-- =========================
REVOKE SELECT ON TABLE public.runs FROM ak_worker;
REVOKE SELECT ON TABLE public.run_inputs FROM ak_worker;
REVOKE SELECT ON TABLE public.run_events FROM ak_worker;
REVOKE SELECT ON TABLE public.run_evidence_assets FROM ak_worker;

COMMIT;