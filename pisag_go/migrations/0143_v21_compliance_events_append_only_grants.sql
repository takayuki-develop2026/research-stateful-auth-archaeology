BEGIN;

-- ============================================================
-- v21: compliance_events_v21 append-only hardening (DDL)
-- ============================================================

-- 0) Safety: ensure table exists
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema='public' AND table_name='compliance_events_v21'
  ) THEN
    RAISE EXCEPTION 'missing table public.compliance_events_v21';
  END IF;
END$$;

-- 1) Remove all default privileges from PUBLIC
REVOKE ALL ON TABLE public.compliance_events_v21 FROM PUBLIC;

-- 2) Enforce: no direct UPDATE/DELETE from typical service roles
-- NOTE: role names vary by deployment. Adjust list if you introduce new roles.
-- If you only have role "ak", this will still prevent accidental DML via granted privileges.
REVOKE UPDATE, DELETE, TRUNCATE ON TABLE public.compliance_events_v21 FROM ak;
-- If you have other roles, uncomment as needed:
-- REVOKE UPDATE, DELETE, TRUNCATE ON TABLE public.compliance_events_v21 FROM ak_worker;
-- REVOKE UPDATE, DELETE, TRUNCATE ON TABLE public.compliance_events_v21 FROM opagateway;
-- REVOKE UPDATE, DELETE, TRUNCATE ON TABLE public.compliance_events_v21 FROM runschedsvc;
-- REVOKE UPDATE, DELETE, TRUNCATE ON TABLE public.compliance_events_v21 FROM decisioncoresvc;
-- REVOKE UPDATE, DELETE, TRUNCATE ON TABLE public.compliance_events_v21 FROM opssvc;

-- 3) Block direct INSERT too (must go through exec-only fn)
REVOKE INSERT ON TABLE public.compliance_events_v21 FROM ak;
-- Uncomment for other roles if needed:
-- REVOKE INSERT ON TABLE public.compliance_events_v21 FROM ak_worker;
-- REVOKE INSERT ON TABLE public.compliance_events_v21 FROM opagateway;

-- 4) SELECT policy (choose one)
-- Option A (strict): allow SELECT only to owner/admin role (keep system minimal)
GRANT SELECT ON TABLE public.compliance_events_v21 TO ak;

-- Option B (if you add a read-only role):
-- CREATE ROLE compliance_reader NOLOGIN;  -- done outside migrations if you prefer
-- GRANT SELECT ON TABLE public.compliance_events_v21 TO compliance_reader;

-- 5) Function is the only write surface: ensure only EXECUTE is granted
-- Existing v21 migration created compliance_event_append_v21; lock it down.
REVOKE ALL ON FUNCTION public.compliance_event_append_v21(
  character varying,
  text,
  text,
  bigint,
  bigint
) FROM PUBLIC;

-- Grant EXECUTE to the role(s) that are allowed to append compliance events
GRANT EXECUTE ON FUNCTION public.compliance_event_append_v21(
  character varying,
  text,
  text,
  bigint,
  bigint
) TO ak;
-- Uncomment if you introduce dedicated service roles:
-- GRANT EXECUTE ON FUNCTION public.compliance_event_append_v21(character varying,text,text,bigint,bigint) TO opagateway;
-- GRANT EXECUTE ON FUNCTION public.compliance_event_append_v21(character varying,text,text,bigint,bigint) TO decisioncoresvc;

-- 6) Optional: add a trigger to hard-block UPDATE/DELETE even for accident (defense-in-depth)
-- This is useful if "ak" remains table owner (owner can bypass grants).
-- If you keep owner as "ak", this trigger is strongly recommended.

CREATE OR REPLACE FUNCTION public._deny_compliance_events_v21_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'compliance_events_v21 is append-only (UPDATE/DELETE forbidden)';
END;
$$;

DROP TRIGGER IF EXISTS trg_deny_update_compliance_events_v21 ON public.compliance_events_v21;
CREATE TRIGGER trg_deny_update_compliance_events_v21
BEFORE UPDATE ON public.compliance_events_v21
FOR EACH ROW EXECUTE FUNCTION public._deny_compliance_events_v21_mutation();

DROP TRIGGER IF EXISTS trg_deny_delete_compliance_events_v21 ON public.compliance_events_v21;
CREATE TRIGGER trg_deny_delete_compliance_events_v21
BEFORE DELETE ON public.compliance_events_v21
FOR EACH ROW EXECUTE FUNCTION public._deny_compliance_events_v21_mutation();

COMMIT;