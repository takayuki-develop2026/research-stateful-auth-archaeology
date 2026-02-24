-- migrations/0029_v18_run_links_fns.sql
-- v18: link tables write-through DB functions (idempotent, exact found_existing)
--
-- Targets:
-- - public.run_evidence_ref_links   (v18 run<->evidence_ref links)  ★IMPORTANT: NOT v4.5 run_evidence_links
-- - public.run_artifact_links       (v18 run<->artifact links)
-- - public.artifact_evidence_links  (v18 artifact<->evidence links)
--
-- Notes:
-- - JSONゼロ（リンクは固定メタのみ）
-- - 直INSERT根絶：アプリはこの関数だけを叩く
-- - idempotent: UNIQUE制約 + DO NOTHING + readback
-- - found_existing is EXACT (pre-check)
--
-- Depends:
-- - public.projects(id)
-- - public.runs(run_id, project_id)
-- - public.evidence_assets(project_id, evidence_ref)
-- - public.artifact_assets(project_id, artifact_ref)
-- - public.run_evidence_ref_links / public.run_artifact_links / public.artifact_evidence_links exist
--
BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- =========================================================
-- helpers
-- =========================================================
CREATE OR REPLACE FUNCTION public._v18_trim_nonempty(p text, p_name text)
RETURNS text
LANGUAGE plpgsql
AS $$
DECLARE
  v text;
BEGIN
  v := btrim(coalesce(p, ''));
  IF v = '' THEN
    RAISE EXCEPTION '% is required', p_name USING ERRCODE='22023';
  END IF;
  RETURN v;
END$$;

-- =========================================================
-- run_evidence_link_add_v18  (v18)
-- writes to public.run_evidence_ref_links
-- =========================================================
CREATE OR REPLACE FUNCTION public.run_evidence_link_add_v18(
  p_project_id      varchar,
  p_run_id          uuid,
  p_evidence_ref    uuid,
  p_role            varchar,
  p_idempotency_key text -- reserved (v13統合用). v18では未使用
)
RETURNS TABLE (
  link_id        bigint,
  found_existing boolean
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id varchar(26);
  v_role       varchar(24);
  v_id         bigint;
BEGIN
  v_project_id := public._v18_trim_nonempty(p_project_id::text, 'project_id')::varchar(26);

  IF p_run_id IS NULL THEN
    RAISE EXCEPTION 'run_id is required' USING ERRCODE='22023';
  END IF;
  IF p_evidence_ref IS NULL THEN
    RAISE EXCEPTION 'evidence_ref is required' USING ERRCODE='22023';
  END IF;

  v_role := public._v18_trim_nonempty(p_role::text, 'role')::varchar(24);
  IF v_role NOT IN ('input','fetched','uploaded','generated','webhook_source') THEN
    RAISE EXCEPTION 'role must be input|fetched|uploaded|generated|webhook_source' USING ERRCODE='22023';
  END IF;

  -- Fast path: already exists?
  SELECT id INTO v_id
  FROM public.run_evidence_ref_links
  WHERE project_id=v_project_id
    AND run_id=p_run_id
    AND evidence_ref=p_evidence_ref
    AND role=v_role
  LIMIT 1;

  IF v_id IS NOT NULL THEN
    link_id := v_id;
    found_existing := true;
    RETURN NEXT;
    RETURN;
  END IF;

  -- Insert (race-safe due to UNIQUE)
  INSERT INTO public.run_evidence_ref_links (project_id, run_id, evidence_ref, role, created_at)
  VALUES (v_project_id, p_run_id, p_evidence_ref, v_role, now())
  ON CONFLICT (project_id, run_id, evidence_ref, role) DO NOTHING;

  -- Read back (must exist now)
  SELECT id INTO v_id
  FROM public.run_evidence_ref_links
  WHERE project_id=v_project_id
    AND run_id=p_run_id
    AND evidence_ref=p_evidence_ref
    AND role=v_role
  LIMIT 1;

  IF v_id IS NULL THEN
    RAISE EXCEPTION 'failed to insert run_evidence_ref_link' USING ERRCODE='23505';
  END IF;

  link_id := v_id;
  found_existing := false;
  RETURN NEXT;
END$$;

REVOKE ALL ON FUNCTION public.run_evidence_link_add_v18(varchar, uuid, uuid, varchar, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.run_evidence_link_add_v18(varchar, uuid, uuid, varchar, text) TO ak;

-- =========================================================
-- run_artifact_link_add_v18
-- =========================================================
CREATE OR REPLACE FUNCTION public.run_artifact_link_add_v18(
  p_project_id      varchar,
  p_run_id          uuid,
  p_artifact_ref    uuid,
  p_role            varchar,
  p_idempotency_key text -- reserved (v13統合用). v18では未使用
)
RETURNS TABLE (
  link_id        bigint,
  found_existing boolean
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id varchar(26);
  v_role       varchar(24);
  v_id         bigint;
BEGIN
  v_project_id := public._v18_trim_nonempty(p_project_id::text, 'project_id')::varchar(26);

  IF p_run_id IS NULL THEN
    RAISE EXCEPTION 'run_id is required' USING ERRCODE='22023';
  END IF;
  IF p_artifact_ref IS NULL THEN
    RAISE EXCEPTION 'artifact_ref is required' USING ERRCODE='22023';
  END IF;

  v_role := public._v18_trim_nonempty(p_role::text, 'role')::varchar(24);
  IF v_role NOT IN ('primary_output','secondary_output','debug_output') THEN
    RAISE EXCEPTION 'role must be primary_output|secondary_output|debug_output' USING ERRCODE='22023';
  END IF;

  SELECT id INTO v_id
  FROM public.run_artifact_links
  WHERE project_id=v_project_id
    AND run_id=p_run_id
    AND artifact_ref=p_artifact_ref
    AND role=v_role
  LIMIT 1;

  IF v_id IS NOT NULL THEN
    link_id := v_id;
    found_existing := true;
    RETURN NEXT;
    RETURN;
  END IF;

  INSERT INTO public.run_artifact_links (project_id, run_id, artifact_ref, role, created_at)
  VALUES (v_project_id, p_run_id, p_artifact_ref, v_role, now())
  ON CONFLICT (project_id, run_id, artifact_ref, role) DO NOTHING;

  SELECT id INTO v_id
  FROM public.run_artifact_links
  WHERE project_id=v_project_id
    AND run_id=p_run_id
    AND artifact_ref=p_artifact_ref
    AND role=v_role
  LIMIT 1;

  IF v_id IS NULL THEN
    RAISE EXCEPTION 'failed to insert run_artifact_link' USING ERRCODE='23505';
  END IF;

  link_id := v_id;
  found_existing := false;
  RETURN NEXT;
END$$;

REVOKE ALL ON FUNCTION public.run_artifact_link_add_v18(varchar, uuid, uuid, varchar, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.run_artifact_link_add_v18(varchar, uuid, uuid, varchar, text) TO ak;

-- =========================================================
-- artifact_evidence_link_add_v18
-- =========================================================
CREATE OR REPLACE FUNCTION public.artifact_evidence_link_add_v18(
  p_project_id      varchar,
  p_artifact_ref    uuid,
  p_evidence_ref    uuid,
  p_link_role       varchar,
  p_idempotency_key text -- reserved (v13統合用). v18では未使用
)
RETURNS TABLE (
  link_id        bigint,
  found_existing boolean
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id varchar(26);
  v_role       varchar(16);
  v_id         bigint;
BEGIN
  v_project_id := public._v18_trim_nonempty(p_project_id::text, 'project_id')::varchar(26);

  IF p_artifact_ref IS NULL THEN
    RAISE EXCEPTION 'artifact_ref is required' USING ERRCODE='22023';
  END IF;
  IF p_evidence_ref IS NULL THEN
    RAISE EXCEPTION 'evidence_ref is required' USING ERRCODE='22023';
  END IF;

  v_role := public._v18_trim_nonempty(p_link_role::text, 'link_role')::varchar(16);
  IF v_role NOT IN ('input','intermediate','supporting','output_proof') THEN
    RAISE EXCEPTION 'link_role must be input|intermediate|supporting|output_proof' USING ERRCODE='22023';
  END IF;

  SELECT id INTO v_id
  FROM public.artifact_evidence_links
  WHERE project_id=v_project_id
    AND artifact_ref=p_artifact_ref
    AND evidence_ref=p_evidence_ref
    AND link_role=v_role
  LIMIT 1;

  IF v_id IS NOT NULL THEN
    link_id := v_id;
    found_existing := true;
    RETURN NEXT;
    RETURN;
  END IF;

  INSERT INTO public.artifact_evidence_links (project_id, artifact_ref, evidence_ref, link_role, created_at)
  VALUES (v_project_id, p_artifact_ref, p_evidence_ref, v_role, now())
  ON CONFLICT (project_id, artifact_ref, evidence_ref, link_role) DO NOTHING;

  SELECT id INTO v_id
  FROM public.artifact_evidence_links
  WHERE project_id=v_project_id
    AND artifact_ref=p_artifact_ref
    AND evidence_ref=p_evidence_ref
    AND link_role=v_role
  LIMIT 1;

  IF v_id IS NULL THEN
    RAISE EXCEPTION 'failed to insert artifact_evidence_link' USING ERRCODE='23505';
  END IF;

  link_id := v_id;
  found_existing := false;
  RETURN NEXT;
END$$;

REVOKE ALL ON FUNCTION public.artifact_evidence_link_add_v18(varchar, uuid, uuid, varchar, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.artifact_evidence_link_add_v18(varchar, uuid, uuid, varchar, text) TO ak;

COMMIT;