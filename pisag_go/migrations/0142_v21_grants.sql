BEGIN;

-- Tables: revoke public
REVOKE ALL ON TABLE public.policy_bundles_v21 FROM PUBLIC;
REVOKE ALL ON TABLE public.policy_decisions_v21 FROM PUBLIC;
REVOKE ALL ON TABLE public.api_keys_v21 FROM PUBLIC;
REVOKE ALL ON TABLE public.service_identities_v21 FROM PUBLIC;
REVOKE ALL ON TABLE public.data_access_rules_v21 FROM PUBLIC;
REVOKE ALL ON TABLE public.compliance_events_v21 FROM PUBLIC;
REVOKE ALL ON TABLE public.privilege_grants_v21 FROM PUBLIC;
REVOKE ALL ON TABLE public.key_rotation_plans_v21 FROM PUBLIC;

-- Minimal readable grants to ak (services use ak in your env)
GRANT SELECT ON TABLE public.policy_bundles_v21 TO ak;
GRANT SELECT ON TABLE public.policy_decisions_v21 TO ak;
GRANT SELECT ON TABLE public.api_keys_v21 TO ak;
GRANT SELECT ON TABLE public.service_identities_v21 TO ak;
GRANT SELECT ON TABLE public.data_access_rules_v21 TO ak;
GRANT SELECT ON TABLE public.compliance_events_v21 TO ak;
GRANT SELECT ON TABLE public.privilege_grants_v21 TO ak;
GRANT SELECT ON TABLE public.key_rotation_plans_v21 TO ak;

-- Functions: revoke public + grant execute
REVOKE ALL ON FUNCTION public.policy_decision_append_v21(
  varchar, text, text, uuid, text, text, text, text, text, text, text,
  bigint, bigint, bigint, bigint, bigint
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.compliance_event_append_v21(
  varchar, text, text, bigint, bigint
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.api_key_create_v21(
  varchar, varchar, text, bigint, timestamptz, varchar, varchar
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.api_key_revoke_v21(
  varchar, varchar, bigint, varchar, varchar
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.privilege_grant_v21(
  varchar, text, text, text, bigint, bigint, text
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.privilege_revoke_v21(
  varchar, bigint, text, bigint
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.key_rotation_plan_create_v21(
  varchar, text, text, bigint, text
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.key_rotation_plan_mark_v21(
  varchar, bigint, text, bigint
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.policy_decision_append_v21(
  varchar, text, text, uuid, text, text, text, text, text, text, text,
  bigint, bigint, bigint, bigint, bigint
) TO ak;

GRANT EXECUTE ON FUNCTION public.compliance_event_append_v21(
  varchar, text, text, bigint, bigint
) TO ak;

GRANT EXECUTE ON FUNCTION public.api_key_create_v21(
  varchar, varchar, text, bigint, timestamptz, varchar, varchar
) TO ak;

GRANT EXECUTE ON FUNCTION public.api_key_revoke_v21(
  varchar, varchar, bigint, varchar, varchar
) TO ak;

GRANT EXECUTE ON FUNCTION public.privilege_grant_v21(
  varchar, text, text, text, bigint, bigint, text
) TO ak;

GRANT EXECUTE ON FUNCTION public.privilege_revoke_v21(
  varchar, bigint, text, bigint
) TO ak;

GRANT EXECUTE ON FUNCTION public.key_rotation_plan_create_v21(
  varchar, text, text, bigint, text
) TO ak;

GRANT EXECUTE ON FUNCTION public.key_rotation_plan_mark_v21(
  varchar, bigint, text, bigint
) TO ak;

COMMIT;