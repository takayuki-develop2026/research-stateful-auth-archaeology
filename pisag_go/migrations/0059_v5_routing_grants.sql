-- migrations/0059_v5_routing_grants.sql
-- v5: grants for routing tables
BEGIN;

REVOKE ALL ON TABLE public.providers FROM PUBLIC;
REVOKE ALL ON TABLE public.provider_routes FROM PUBLIC;
REVOKE ALL ON TABLE public.route_decisions FROM PUBLIC;
REVOKE ALL ON TABLE public.routing_metrics_daily FROM PUBLIC;

-- API / usecase role
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.providers TO ak;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.provider_routes TO ak;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.route_decisions TO ak;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.routing_metrics_daily TO ak;

-- worker role (read + metrics write only)
GRANT SELECT ON TABLE public.providers TO ak_worker;
GRANT SELECT ON TABLE public.provider_routes TO ak_worker;
GRANT SELECT ON TABLE public.route_decisions TO ak_worker;
GRANT SELECT, INSERT, UPDATE ON TABLE public.routing_metrics_daily TO ak_worker;

COMMIT;