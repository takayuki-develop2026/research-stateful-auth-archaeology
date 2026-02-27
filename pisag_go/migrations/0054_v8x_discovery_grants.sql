-- migrations/0054_v8x_discovery_grants.sql
-- v8.x: minimal grants for discovery tables
-- Policy: revoke from PUBLIC; allow ak (API/usecase) read/write; worker read/write only what needed.
-- NOTE: If you want EXECUTE ONLY via functions, we can add v8 functions later (v8.1).

BEGIN;

-- Revoke from PUBLIC
REVOKE ALL ON TABLE public.discovery_sources FROM PUBLIC;
REVOKE ALL ON TABLE public.discovery_candidates FROM PUBLIC;
REVOKE ALL ON TABLE public.discovery_candidate_events FROM PUBLIC;

REVOKE ALL ON TABLE public.dedupe_groups FROM PUBLIC;
REVOKE ALL ON TABLE public.dedupe_group_members FROM PUBLIC;
REVOKE ALL ON TABLE public.dedupe_decisions FROM PUBLIC;

REVOKE ALL ON TABLE public.discovery_candidate_lifecycle_events FROM PUBLIC;
REVOKE ALL ON TABLE public.discovery_lifecycle_jobs FROM PUBLIC;

-- API/usecase role (ak) - full access (initially)
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.discovery_sources TO ak;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.discovery_candidates TO ak;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.discovery_candidate_events TO ak;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.dedupe_groups TO ak;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.dedupe_group_members TO ak;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.dedupe_decisions TO ak;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.discovery_candidate_lifecycle_events TO ak;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.discovery_lifecycle_jobs TO ak;

-- sequences (bigserial)
GRANT USAGE, SELECT ON SEQUENCE public.discovery_sources_id_seq TO ak;
GRANT USAGE, SELECT ON SEQUENCE public.discovery_candidates_id_seq TO ak;
GRANT USAGE, SELECT ON SEQUENCE public.discovery_candidate_events_id_seq TO ak;

GRANT USAGE, SELECT ON SEQUENCE public.dedupe_groups_id_seq TO ak;
GRANT USAGE, SELECT ON SEQUENCE public.dedupe_group_members_id_seq TO ak;
GRANT USAGE, SELECT ON SEQUENCE public.dedupe_decisions_id_seq TO ak;

GRANT USAGE, SELECT ON SEQUENCE public.discovery_candidate_lifecycle_events_id_seq TO ak;
GRANT USAGE, SELECT ON SEQUENCE public.discovery_lifecycle_jobs_id_seq TO ak;

-- worker role (ak_worker): read + update lifecycle scheduling fields (tighten later if needed)
GRANT SELECT ON TABLE public.discovery_sources TO ak_worker;
GRANT SELECT ON TABLE public.discovery_candidates TO ak_worker;
GRANT SELECT ON TABLE public.dedupe_groups TO ak_worker;

GRANT INSERT ON TABLE public.discovery_candidate_lifecycle_events TO ak_worker;
GRANT INSERT, UPDATE ON TABLE public.discovery_lifecycle_jobs TO ak_worker;

GRANT USAGE, SELECT ON SEQUENCE public.discovery_candidate_lifecycle_events_id_seq TO ak_worker;
GRANT USAGE, SELECT ON SEQUENCE public.discovery_lifecycle_jobs_id_seq TO ak_worker;

COMMIT;