BEGIN;

-- ============================================================
-- v21: compliance_events_v21 append-only enforcement (owner-safe)
--  - Block direct INSERT/UPDATE/DELETE (even for owner)
--  - Allow INSERT only when called via compliance_event_append_v21
--  - Treat primary_artifact_asset_id=0 as NULL
-- ============================================================

-- 0) Ensure pgcrypto (for gen_random_uuid etc. if needed)
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 1) Insert-guard trigger: only allow INSERT when session flag is set
CREATE OR REPLACE FUNCTION public._deny_compliance_events_v21_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  v_allow text;
BEGIN
  -- session-local flag set by SECURITY DEFINER function only
  v_allow := current_setting('compliance_v21.allow_insert', true);

  IF v_allow IS DISTINCT FROM '1' THEN
    RAISE EXCEPTION 'compliance_events_v21 is append-only via exec-only function (direct INSERT forbidden)';
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_deny_insert_compliance_events_v21 ON public.compliance_events_v21;
CREATE TRIGGER trg_deny_insert_compliance_events_v21
BEFORE INSERT ON public.compliance_events_v21
FOR EACH ROW EXECUTE FUNCTION public._deny_compliance_events_v21_insert();


-- 2) UPDATE/DELETE guard trigger (you already added, keep it)
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


-- 3) Fix exec-only function: set flag + treat 0 as NULL
--    (signature you confirmed: (varchar, text, text, bigint, bigint) returns table(event_id bigint))
CREATE OR REPLACE FUNCTION public.compliance_event_append_v21(
  p_project_id character varying,
  p_trace_id text,
  p_event_type text,
  p_event_evidence_asset_id bigint,
  p_primary_artifact_asset_id bigint
)
RETURNS TABLE(event_id bigint)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id varchar(26);
  v_primary_artifact_id bigint;
BEGIN
  v_project_id := btrim(coalesce(p_project_id::text,''))::varchar(26);
  IF v_project_id IS NULL OR btrim(v_project_id::text) = '' THEN
    RAISE EXCEPTION 'project_id is required';
  END IF;

  IF btrim(coalesce(p_trace_id,'')) = '' THEN
    RAISE EXCEPTION 'trace_id is required';
  END IF;

  IF btrim(coalesce(p_event_type,'')) = '' THEN
    RAISE EXCEPTION 'event_type is required';
  END IF;

  IF p_event_evidence_asset_id IS NULL OR p_event_evidence_asset_id <= 0 THEN
    RAISE EXCEPTION 'event_evidence_asset_id is required';
  END IF;

  -- 0 => NULL (important: avoid FK to artifact_assets)
  v_primary_artifact_id := NULLIF(p_primary_artifact_asset_id, 0);

  -- Set session-local flag so INSERT trigger allows this insert only
  PERFORM set_config('compliance_v21.allow_insert', '1', true);

  INSERT INTO public.compliance_events_v21(
    project_id,
    trace_id,
    event_type,
    event_evidence_asset_id,
    primary_artifact_asset_id,
    created_at_utc
  )
  VALUES (
    v_project_id,
    btrim(coalesce(p_trace_id,'')),
    btrim(coalesce(p_event_type,'')),
    p_event_evidence_asset_id,
    v_primary_artifact_id,
    now()
  )
  RETURNING id INTO event_id;

  RETURN NEXT;
END;
$$;

REVOKE ALL ON FUNCTION public.compliance_event_append_v21(
  character varying, text, text, bigint, bigint
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.compliance_event_append_v21(
  character varying, text, text, bigint, bigint
) TO ak;

COMMIT;