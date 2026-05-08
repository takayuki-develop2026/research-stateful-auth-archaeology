-- migrations/0000_roles_v41.sql (safe / existence-guarded)
BEGIN;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_roles
    WHERE rolname = 'ak_worker'
  ) THEN
    CREATE ROLE ak_worker LOGIN PASSWORD 'ak_worker';
  END IF;
END
$$;

GRANT USAGE ON SCHEMA public TO ak_worker;

REVOKE ALL ON ALL TABLES IN SCHEMA public FROM ak_worker;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM ak_worker;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = 'public'
      AND table_name = 'runs'
  ) THEN
    GRANT INSERT ON TABLE public.runs TO ak_worker;
    REVOKE SELECT ON TABLE public.runs FROM ak_worker;
  END IF;

  IF EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = 'public'
      AND table_name = 'run_inputs'
  ) THEN
    GRANT INSERT ON TABLE public.run_inputs TO ak_worker;
    REVOKE SELECT ON TABLE public.run_inputs FROM ak_worker;
  END IF;

  IF EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = 'public'
      AND table_name = 'run_events'
  ) THEN
    GRANT INSERT ON TABLE public.run_events TO ak_worker;
    REVOKE SELECT ON TABLE public.run_events FROM ak_worker;
  END IF;

  IF EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = 'public'
      AND table_name = 'run_evidence_assets'
  ) THEN
    GRANT INSERT ON TABLE public.run_evidence_assets TO ak_worker;
    REVOKE SELECT ON TABLE public.run_evidence_assets FROM ak_worker;
  END IF;
END
$$;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public'
      AND c.relkind = 'S'
      AND c.relname = 'run_inputs_id_seq'
  ) THEN
    GRANT USAGE, SELECT ON SEQUENCE public.run_inputs_id_seq TO ak_worker;
  END IF;

  IF EXISTS (
    SELECT 1
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public'
      AND c.relkind = 'S'
      AND c.relname = 'run_events_id_seq'
  ) THEN
    GRANT USAGE, SELECT ON SEQUENCE public.run_events_id_seq TO ak_worker;
  END IF;

  IF EXISTS (
    SELECT 1
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public'
      AND c.relkind = 'S'
      AND c.relname = 'run_evidence_assets_id_seq'
  ) THEN
    GRANT USAGE, SELECT ON SEQUENCE public.run_evidence_assets_id_seq TO ak_worker;
  END IF;
END
$$;

COMMIT;
