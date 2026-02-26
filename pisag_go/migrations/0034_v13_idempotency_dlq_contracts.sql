-- migrations/0034_v13_idempotency_dlq_contracts.sql
-- v13 P0: Idempotency / DLQ / Compat Contracts (EXECUTE ONLY)
-- Evidence reference: evidence_assets.id (bigint)
-- Roles:
--   - PUBLIC: no execute
--   - ak: execute allowed (admin/API side)
--   - ak_worker: no direct privileges for v13 tables (P0)

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================
-- 1) Tables
-- ============================================================

-- 1.1 idempotency_records_v13
CREATE TABLE IF NOT EXISTS public.idempotency_records_v13 (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  scope varchar(64) NOT NULL,
  idempotency_key varchar(128) NOT NULL, -- namespace required by policy
  request_fingerprint char(64) NULL,     -- internal hash ok

  status varchar(16) NOT NULL, -- started|succeeded|review_required|failed
  result_summary varchar(256) NULL,

  result_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  started_at timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz NULL,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT idem_scope_nonempty CHECK (btrim(scope::text) <> ''),
  CONSTRAINT idem_key_nonempty CHECK (btrim(idempotency_key::text) <> ''),
  CONSTRAINT idem_status_valid CHECK (lower(status::text) IN ('started','succeeded','review_required','failed'))
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_idem_v13') THEN
    ALTER TABLE public.idempotency_records_v13
      ADD CONSTRAINT ux_idem_v13 UNIQUE (project_id, scope, idempotency_key);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_idem_v13_project_scope_time
  ON public.idempotency_records_v13(project_id, scope, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_idem_v13_project_status_time
  ON public.idempotency_records_v13(project_id, status, started_at DESC);


-- 1.2 dlq_items_v13
CREATE TABLE IF NOT EXISTS public.dlq_items_v13 (
  dlq_id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  run_id uuid NULL REFERENCES public.runs(run_id) ON DELETE SET NULL,
  trace_id uuid NOT NULL,

  task_type varchar(64) NOT NULL,
  source varchar(16) NOT NULL, -- queue|scheduler|webhook|manual
  correlation_key varchar(128) NULL,

  payload_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  last_error_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  attempts int NOT NULL DEFAULT 0,
  status varchar(16) NOT NULL DEFAULT 'pending', -- pending|requeued|resolved|ignored

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT dlq_task_nonempty CHECK (btrim(task_type::text) <> ''),
  CONSTRAINT dlq_source_valid CHECK (lower(source::text) IN ('queue','scheduler','webhook','manual')),
  CONSTRAINT dlq_status_valid CHECK (lower(status::text) IN ('pending','requeued','resolved','ignored')),
  CONSTRAINT dlq_attempts_nonneg CHECK (attempts >= 0)
);

CREATE INDEX IF NOT EXISTS idx_dlq_v13_project_status_time
  ON public.dlq_items_v13(project_id, status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_dlq_v13_project_task_time
  ON public.dlq_items_v13(project_id, task_type, updated_at DESC);


-- 1.3 compat_contracts_v13
CREATE TABLE IF NOT EXISTS public.compat_contracts_v13 (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  contract_type varchar(32) NOT NULL,    -- openapi|db_schema|policy_bundle|key_spec
  contract_version varchar(16) NOT NULL, -- v1 etc
  checksum_sha256 char(64) NOT NULL,

  artifact_ref varchar(256) NULL, -- pointer, not evidence
  diff_summary varchar(256) NULL,
  detail_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT cc_type_nonempty CHECK (btrim(contract_type::text) <> ''),
  CONSTRAINT cc_ver_nonempty CHECK (btrim(contract_version::text) <> ''),
  CONSTRAINT cc_sha_len CHECK (length(checksum_sha256) = 64)
);

CREATE INDEX IF NOT EXISTS idx_cc_v13_project_type_time
  ON public.compat_contracts_v13(project_id, contract_type, created_at DESC);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_cc_v13') THEN
    ALTER TABLE public.compat_contracts_v13
      ADD CONSTRAINT ux_cc_v13 UNIQUE (project_id, contract_type, contract_version, checksum_sha256);
  END IF;
END$$;


-- ============================================================
-- 2) SECURITY DEFINER functions (EXECUTE ONLY)
-- ============================================================

-- 2.1 idempotency_start_v13
CREATE OR REPLACE FUNCTION public.idempotency_start_v13(
  p_project_id varchar,
  p_scope varchar,
  p_idempotency_key varchar,
  p_request_fingerprint char(64)
)
RETURNS TABLE(idempotency_id bigint, found_existing boolean)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_scope text := btrim(coalesce(p_scope::text,''));
  v_key text := btrim(coalesce(p_idempotency_key::text,''));
  v_existing bigint;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_scope='' THEN RAISE EXCEPTION 'scope required' USING ERRCODE='22023'; END IF;
  IF v_key='' THEN RAISE EXCEPTION 'idempotency_key required' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects WHERE id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  SELECT id INTO v_existing
  FROM public.idempotency_records_v13
  WHERE project_id=v_project_id::varchar(26) AND scope=v_scope AND idempotency_key=v_key
  LIMIT 1;

  IF v_existing IS NOT NULL THEN
    idempotency_id := v_existing;
    found_existing := true;
    RETURN NEXT; RETURN;
  END IF;

  INSERT INTO public.idempotency_records_v13(
    project_id, scope, idempotency_key, request_fingerprint, status, started_at, created_at, updated_at
  )
  VALUES (
    v_project_id::varchar(26), v_scope, v_key, p_request_fingerprint, 'started', now(), now(), now()
  )
  RETURNING id INTO v_existing;

  idempotency_id := v_existing;
  found_existing := false;
  RETURN NEXT;
END;
$$;

-- 2.2 idempotency_finish_v13
CREATE OR REPLACE FUNCTION public.idempotency_finish_v13(
  p_project_id varchar,
  p_idempotency_id bigint,
  p_status varchar, -- succeeded|review_required|failed
  p_result_summary varchar,
  p_result_evidence_asset_id bigint
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_status text := lower(btrim(coalesce(p_status::text,'')));
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF p_idempotency_id IS NULL OR p_idempotency_id<=0 THEN RAISE EXCEPTION 'idempotency_id required' USING ERRCODE='22023'; END IF;
  IF v_status NOT IN ('succeeded','review_required','failed') THEN
    RAISE EXCEPTION 'status must be succeeded|review_required|failed' USING ERRCODE='22023';
  END IF;

  IF p_result_evidence_asset_id IS NOT NULL THEN
    PERFORM 1 FROM public.evidence_assets WHERE id=p_result_evidence_asset_id;
    IF NOT FOUND THEN RAISE EXCEPTION 'evidence_asset not found: %', p_result_evidence_asset_id USING ERRCODE='23503'; END IF;
  END IF;

  UPDATE public.idempotency_records_v13
  SET status=v_status::varchar(16),
      result_summary=NULLIF(btrim(coalesce(p_result_summary::text,'')),'')::varchar(256),
      result_evidence_asset_id=p_result_evidence_asset_id,
      finished_at=now(),
      updated_at=now()
  WHERE id=p_idempotency_id AND project_id=v_project_id::varchar(26);

  IF NOT FOUND THEN
    RAISE EXCEPTION 'idempotency record not found' USING ERRCODE='23503';
  END IF;
END;
$$;

-- 2.3 dlq_enqueue_v13
CREATE OR REPLACE FUNCTION public.dlq_enqueue_v13(
  p_project_id varchar,
  p_run_id uuid,
  p_trace_id uuid,
  p_task_type varchar,
  p_source varchar,
  p_correlation_key varchar,
  p_payload_evidence_asset_id bigint,
  p_last_error_evidence_asset_id bigint
)
RETURNS bigint
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_source text := lower(btrim(coalesce(p_source::text,'')));
  v_task text := btrim(coalesce(p_task_type::text,''));
  v_id bigint;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF p_trace_id IS NULL THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;
  IF v_task='' THEN RAISE EXCEPTION 'task_type required' USING ERRCODE='22023'; END IF;
  IF v_source NOT IN ('queue','scheduler','webhook','manual') THEN RAISE EXCEPTION 'source invalid' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects WHERE id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  PERFORM 1 FROM public.evidence_assets WHERE id=p_payload_evidence_asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'payload evidence not found' USING ERRCODE='23503'; END IF;

  IF p_last_error_evidence_asset_id IS NOT NULL THEN
    PERFORM 1 FROM public.evidence_assets WHERE id=p_last_error_evidence_asset_id;
    IF NOT FOUND THEN RAISE EXCEPTION 'error evidence not found' USING ERRCODE='23503'; END IF;
  END IF;

  IF p_run_id IS NOT NULL THEN
    PERFORM 1 FROM public.runs WHERE run_id=p_run_id;
    IF NOT FOUND THEN RAISE EXCEPTION 'run not found: %', p_run_id USING ERRCODE='23503'; END IF;
  END IF;

  INSERT INTO public.dlq_items_v13(
    project_id, run_id, trace_id, task_type, source, correlation_key,
    payload_evidence_asset_id, last_error_evidence_asset_id,
    attempts, status, created_at, updated_at
  )
  VALUES (
    v_project_id::varchar(26), p_run_id, p_trace_id,
    v_task::varchar(64), v_source::varchar(16),
    NULLIF(btrim(coalesce(p_correlation_key::text,'')),'')::varchar(128),
    p_payload_evidence_asset_id, p_last_error_evidence_asset_id,
    0, 'pending', now(), now()
  )
  RETURNING dlq_id INTO v_id;

  RETURN v_id;
END;
$$;

-- 2.4 dlq_mark_v13 (requeue/resolve/ignored)
CREATE OR REPLACE FUNCTION public.dlq_mark_v13(
  p_project_id varchar,
  p_dlq_id bigint,
  p_status varchar, -- requeued|resolved|ignored
  p_result_error_evidence_asset_id bigint
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_status text := lower(btrim(coalesce(p_status::text,'')));
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF p_dlq_id IS NULL OR p_dlq_id<=0 THEN RAISE EXCEPTION 'dlq_id required' USING ERRCODE='22023'; END IF;
  IF v_status NOT IN ('requeued','resolved','ignored') THEN
    RAISE EXCEPTION 'status must be requeued|resolved|ignored' USING ERRCODE='22023';
  END IF;

  IF p_result_error_evidence_asset_id IS NOT NULL THEN
    PERFORM 1 FROM public.evidence_assets WHERE id=p_result_error_evidence_asset_id;
    IF NOT FOUND THEN RAISE EXCEPTION 'error evidence not found' USING ERRCODE='23503'; END IF;
  END IF;

  UPDATE public.dlq_items_v13
  SET status=v_status::varchar(16),
      attempts=CASE WHEN v_status='requeued' THEN attempts+1 ELSE attempts END,
      last_error_evidence_asset_id=COALESCE(p_result_error_evidence_asset_id, last_error_evidence_asset_id),
      updated_at=now()
  WHERE dlq_id=p_dlq_id AND project_id=v_project_id::varchar(26);

  IF NOT FOUND THEN
    RAISE EXCEPTION 'dlq item not found' USING ERRCODE='23503';
  END IF;
END;
$$;

-- 2.5 compat_contract_insert_v13
CREATE OR REPLACE FUNCTION public.compat_contract_insert_v13(
  p_project_id varchar,
  p_contract_type varchar,
  p_contract_version varchar,
  p_checksum_sha256 char(64),
  p_artifact_ref varchar,
  p_diff_summary varchar,
  p_detail_evidence_asset_id bigint
)
RETURNS bigint
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_type text := btrim(coalesce(p_contract_type::text,''));
  v_ver text := btrim(coalesce(p_contract_version::text,''));
  v_id bigint;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_type='' THEN RAISE EXCEPTION 'contract_type required' USING ERRCODE='22023'; END IF;
  IF v_ver='' THEN RAISE EXCEPTION 'contract_version required' USING ERRCODE='22023'; END IF;
  IF p_checksum_sha256 IS NULL OR length(p_checksum_sha256)<>64 THEN RAISE EXCEPTION 'checksum_sha256 must be 64' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects WHERE id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  IF p_detail_evidence_asset_id IS NOT NULL THEN
    PERFORM 1 FROM public.evidence_assets WHERE id=p_detail_evidence_asset_id;
    IF NOT FOUND THEN RAISE EXCEPTION 'detail evidence not found' USING ERRCODE='23503'; END IF;
  END IF;

  INSERT INTO public.compat_contracts_v13(
    project_id, contract_type, contract_version, checksum_sha256,
    artifact_ref, diff_summary, detail_evidence_asset_id, created_at
  )
  VALUES (
    v_project_id::varchar(26),
    v_type::varchar(32),
    v_ver::varchar(16),
    p_checksum_sha256,
    NULLIF(btrim(coalesce(p_artifact_ref::text,'')),'')::varchar(256),
    NULLIF(btrim(coalesce(p_diff_summary::text,'')),'')::varchar(256),
    p_detail_evidence_asset_id,
    now()
  )
  ON CONFLICT (project_id, contract_type, contract_version, checksum_sha256)
  DO UPDATE SET
    artifact_ref=EXCLUDED.artifact_ref,
    diff_summary=EXCLUDED.diff_summary,
    detail_evidence_asset_id=EXCLUDED.detail_evidence_asset_id
  RETURNING id INTO v_id;

  RETURN v_id;
END;
$$;

-- ============================================================
-- 3) Permissions (EXECUTE ONLY)
-- ============================================================

REVOKE ALL ON TABLE public.idempotency_records_v13 FROM PUBLIC;
REVOKE ALL ON TABLE public.dlq_items_v13 FROM PUBLIC;
REVOKE ALL ON TABLE public.compat_contracts_v13 FROM PUBLIC;

REVOKE ALL ON FUNCTION public.idempotency_start_v13(varchar,varchar,varchar,char) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.idempotency_finish_v13(varchar,bigint,varchar,varchar,bigint) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.dlq_enqueue_v13(varchar,uuid,uuid,varchar,varchar,varchar,bigint,bigint) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.dlq_mark_v13(varchar,bigint,varchar,bigint) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.compat_contract_insert_v13(varchar,varchar,varchar,char,varchar,varchar,bigint) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.idempotency_start_v13(varchar,varchar,varchar,char) TO ak;
GRANT EXECUTE ON FUNCTION public.idempotency_finish_v13(varchar,bigint,varchar,varchar,bigint) TO ak;
GRANT EXECUTE ON FUNCTION public.dlq_enqueue_v13(varchar,uuid,uuid,varchar,varchar,varchar,bigint,bigint) TO ak;
GRANT EXECUTE ON FUNCTION public.dlq_mark_v13(varchar,bigint,varchar,bigint) TO ak;
GRANT EXECUTE ON FUNCTION public.compat_contract_insert_v13(varchar,varchar,varchar,char,varchar,varchar,bigint) TO ak;

COMMIT;