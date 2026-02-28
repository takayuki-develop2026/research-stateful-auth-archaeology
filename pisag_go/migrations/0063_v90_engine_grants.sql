-- migrations/0063_v90_engine_grants.sql
-- v9.0: grants for engine router tables
BEGIN;

REVOKE ALL ON TABLE public.engine_runs_v9 FROM PUBLIC;
REVOKE ALL ON TABLE public.decision_ledger_v9 FROM PUBLIC;
REVOKE ALL ON TABLE public.engine_cache_v9 FROM PUBLIC;

-- API / usecase role
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.engine_runs_v9 TO ak;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.decision_ledger_v9 TO ak;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.engine_cache_v9 TO ak;

-- worker role (read-only by default)
GRANT SELECT ON TABLE public.engine_runs_v9 TO ak_worker;
GRANT SELECT ON TABLE public.decision_ledger_v9 TO ak_worker;
GRANT SELECT ON TABLE public.engine_cache_v9 TO ak_worker;

COMMIT;