-- migrations/0000_roles_v41.sql
-- v4.1 runtime sandbox: create ak_worker with minimal privileges
-- Run this as DB owner (ak) / superuser.

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ak_worker') THEN
    CREATE ROLE ak_worker LOGIN PASSWORD 'ak_worker';
  END IF;
END$$;

-- allow using public schema (required for table access)
GRANT USAGE ON SCHEMA public TO ak_worker;

-- v4.1 runtime privileges:
-- runs: INSERT + UPDATE (MarkDone/MarkFailed needs UPDATE)
GRANT INSERT, UPDATE ON TABLE runs TO ak_worker;

-- run_inputs/run_events: INSERT only
GRANT INSERT ON TABLE run_inputs TO ak_worker;
GRANT INSERT ON TABLE run_events TO ak_worker;

-- bigserial sequences need USAGE for nextval()
GRANT USAGE, SELECT ON SEQUENCE run_inputs_id_seq TO ak_worker;
GRANT USAGE, SELECT ON SEQUENCE run_events_id_seq TO ak_worker;

-- Explicitly deny reads (makes intent obvious; harmless if no SELECT was granted)
REVOKE SELECT ON TABLE runs FROM ak_worker;
REVOKE SELECT ON TABLE run_inputs FROM ak_worker;
REVOKE SELECT ON TABLE run_events FROM ak_worker;