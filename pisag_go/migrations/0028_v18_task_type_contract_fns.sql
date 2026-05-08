-- migrations/0028_v18_task_type_contract_fns.sql
-- v18: task_type_contracts upsert/enable/disable as DB functions (write through functions)
-- Notes:
-- - JSONゼロ（契約本文・詳細は evidence_assets を参照）
-- - 変更履歴は contract_change_records
-- - 13引数のcore + 12引数のwrapper(あなたが叩く形)を両立させる

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- =========================================================
-- helper: trim nonempty
-- =========================================================
CREATE OR REPLACE FUNCTION public._nonempty_v18(p text, p_name text)
RETURNS text
LANGUAGE plpgsql
AS $$
DECLARE
  v text;
BEGIN
  v := btrim(coalesce(p, ''));
  IF v = '' THEN
    RAISE EXCEPTION '% is required', p_name USING ERRCODE = '22023';
  END IF;
  RETURN v;
END$$;

-- =========================================================
-- helper: normalize created_by_type
-- =========================================================
CREATE OR REPLACE FUNCTION public._normalize_actor_type_v18(p text)
RETURNS text
LANGUAGE plpgsql
AS $$
DECLARE
  v text;
BEGIN
  v := lower(btrim(coalesce(p, 'system')));
  IF v NOT IN ('system','user','service') THEN
    RAISE EXCEPTION 'created_by_type must be system|user|service' USING ERRCODE='22023';
  END IF;
  RETURN v;
END$$;

-- =========================================================
-- (optional) idempotency (v18 local)
-- 既にあるならそのまま
-- =========================================================
CREATE TABLE IF NOT EXISTS public.task_type_contract_idempotency_v18 (
  id               bigserial PRIMARY KEY,
  project_id       varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  idempotency_key  text        NOT NULL,
  contract_id      bigint      NOT NULL,
  created_at       timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_task_type_contract_idem_v18') THEN
    ALTER TABLE public.task_type_contract_idempotency_v18
      ADD CONSTRAINT ux_task_type_contract_idem_v18 UNIQUE (project_id, idempotency_key);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_task_type_contract_idem_v18_project_time
  ON public.task_type_contract_idempotency_v18(project_id, created_at DESC);

-- =========================================================
-- core: 13 args (あなたの貼った設計を維持)
-- =========================================================
CREATE OR REPLACE FUNCTION public.task_type_contract_upsert_v18(
  p_project_id                   varchar,
  p_task_type                    varchar,
  p_pipeline_version             varchar,
  p_policy_version_id            varchar,     -- nullable allowed
  p_enabled                      boolean,
  p_input_contract_evidence_ref  uuid,
  p_output_contract_evidence_ref uuid,
  p_default_mode                 varchar,     -- nullable allowed
  p_created_by_type              varchar,
  p_created_by_id                varchar,     -- nullable allowed
  p_trace_id                     varchar,
  p_run_id                       uuid,        -- nullable allowed
  p_idempotency_key              text         -- nullable; reserved for v13 integration
)
RETURNS TABLE (
  contract_id    bigint,
  change_kind    varchar(16),
  found_existing boolean
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id   varchar(26);
  v_task_type    varchar(32);
  v_pipe         varchar(64);
  v_trace        varchar(64);
  v_actor_type   varchar(16);
  v_actor_id     varchar(128);
  v_enabled      boolean;

  ex_id          bigint;
  ex_enabled     boolean;
  ex_policy      varchar(26);
  ex_in_ref      uuid;
  ex_out_ref     uuid;
  ex_mode        varchar(16);

  v_kind         varchar(16);
  v_found        boolean := false;

  changed_other  boolean := false;
  changed_flag   boolean := false;
BEGIN
  v_project_id := public._nonempty_v18(p_project_id::text, 'project_id')::varchar(26);
  v_task_type  := public._nonempty_v18(p_task_type::text, 'task_type')::varchar(32);
  v_pipe       := public._nonempty_v18(p_pipeline_version::text, 'pipeline_version')::varchar(64);
  v_trace      := public._nonempty_v18(p_trace_id::text, 'trace_id')::varchar(64);

  v_actor_type := public._normalize_actor_type_v18(p_created_by_type::text)::varchar(16);
  v_actor_id   := NULLIF(btrim(coalesce(p_created_by_id::text, '')), '')::varchar(128);

  v_enabled := coalesce(p_enabled, true);

  IF p_input_contract_evidence_ref IS NULL THEN
    RAISE EXCEPTION 'input_contract_evidence_ref is required' USING ERRCODE='22023';
  END IF;
  IF p_output_contract_evidence_ref IS NULL THEN
    RAISE EXCEPTION 'output_contract_evidence_ref is required' USING ERRCODE='22023';
  END IF;

  -- evidence must exist in project
  PERFORM 1 FROM public.evidence_assets
    WHERE project_id = v_project_id AND evidence_ref = p_input_contract_evidence_ref;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'input_contract_evidence_ref not found in project' USING ERRCODE='23503';
  END IF;

  PERFORM 1 FROM public.evidence_assets
    WHERE project_id = v_project_id AND evidence_ref = p_output_contract_evidence_ref;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'output_contract_evidence_ref not found in project' USING ERRCODE='23503';
  END IF;

  -- idempotency fast-path（同一キーは同じ contract_id を返す）
  IF NULLIF(btrim(coalesce(p_idempotency_key,'')), '') IS NOT NULL THEN
    SELECT t.contract_id INTO ex_id
    FROM public.task_type_contract_idempotency_v18 t
    WHERE t.project_id = v_project_id
      AND t.idempotency_key = p_idempotency_key
    LIMIT 1;

    IF ex_id IS NOT NULL THEN
      contract_id := ex_id;
      change_kind := 'updated';
      found_existing := true;
      RETURN NEXT;
      RETURN;
    END IF;
  END IF;

  -- lock existing row
  SELECT id, enabled, policy_version_id, input_contract_evidence_ref, output_contract_evidence_ref, default_mode
    INTO ex_id, ex_enabled, ex_policy, ex_in_ref, ex_out_ref, ex_mode
  FROM public.task_type_contracts
  WHERE project_id = v_project_id
    AND task_type = v_task_type
    AND pipeline_version = v_pipe
  FOR UPDATE;

  IF NOT FOUND THEN
    INSERT INTO public.task_type_contracts (
      project_id, task_type, pipeline_version,
      policy_version_id, enabled,
      input_contract_evidence_ref, output_contract_evidence_ref,
      default_mode,
      created_by_type, created_by_id,
      created_at, updated_at
    ) VALUES (
      v_project_id, v_task_type, v_pipe,
      NULLIF(btrim(coalesce(p_policy_version_id::text,'')),'')::varchar(26),
      v_enabled,
      p_input_contract_evidence_ref, p_output_contract_evidence_ref,
      NULLIF(btrim(coalesce(p_default_mode::text,'')),'')::varchar(16),
      v_actor_type, v_actor_id,
      now(), now()
    )
    RETURNING id INTO ex_id;

    v_kind := 'created';
    v_found := false;

  ELSE
    v_found := true;

    changed_flag := (ex_enabled IS DISTINCT FROM v_enabled);

    changed_other :=
      (NULLIF(btrim(coalesce(p_policy_version_id::text,'')),'')::varchar(26) IS DISTINCT FROM ex_policy)
      OR (p_input_contract_evidence_ref IS DISTINCT FROM ex_in_ref)
      OR (p_output_contract_evidence_ref IS DISTINCT FROM ex_out_ref)
      OR (NULLIF(btrim(coalesce(p_default_mode::text,'')),'')::varchar(16) IS DISTINCT FROM ex_mode);

    UPDATE public.task_type_contracts
      SET policy_version_id            = NULLIF(btrim(coalesce(p_policy_version_id::text,'')),'')::varchar(26),
          enabled                      = v_enabled,
          input_contract_evidence_ref  = p_input_contract_evidence_ref,
          output_contract_evidence_ref = p_output_contract_evidence_ref,
          default_mode                 = NULLIF(btrim(coalesce(p_default_mode::text,'')),'')::varchar(16),
          created_by_type              = v_actor_type,
          created_by_id                = v_actor_id,
          updated_at                   = now()
    WHERE id = ex_id;

    IF changed_other THEN
      v_kind := 'updated';
    ELSIF changed_flag THEN
      v_kind := CASE WHEN v_enabled THEN 'enabled' ELSE 'disabled' END;
    ELSE
      v_kind := 'updated'; -- no-opでも呼び出しは成功扱い
    END IF;
  END IF;

  -- change record（created / updated / enabled / disabled のみ記録。no-opは抑制）
  IF (NOT v_found) OR changed_flag OR changed_other THEN
    INSERT INTO public.contract_change_records (
      project_id, task_type, pipeline_version,
      change_kind,
      before_contract_evidence_ref,
      after_contract_evidence_ref,
      trace_id,
      run_id,
      created_by_user_id,
      created_at
    )
    SELECT
      v_project_id, v_task_type, v_pipe,
      v_kind,
      CASE WHEN v_found THEN ex_in_ref ELSE NULL END,
      p_input_contract_evidence_ref,
      v_trace,
      p_run_id,
      v_actor_id,
      now()
    WHERE NOT EXISTS (
      SELECT 1
      FROM public.contract_change_records c
      WHERE c.project_id = v_project_id
        AND c.task_type = v_task_type
        AND c.pipeline_version = v_pipe
        AND c.change_kind = v_kind
        AND c.trace_id = v_trace
    );
  END IF;

  -- write idempotency mapping after success
  IF NULLIF(btrim(coalesce(p_idempotency_key,'')), '') IS NOT NULL THEN
    INSERT INTO public.task_type_contract_idempotency_v18(project_id, idempotency_key, contract_id, created_at)
    VALUES (v_project_id, p_idempotency_key, ex_id, now())
    ON CONFLICT (project_id, idempotency_key) DO NOTHING;
  END IF;

  contract_id := ex_id;
  change_kind := v_kind;
  found_existing := v_found;
  RETURN NEXT;
END$$;

-- =========================================================
-- ✅ wrapper: 12 args (あなたが叩きたい形)
-- =========================================================
CREATE OR REPLACE FUNCTION public.task_type_contract_upsert_v18(
  p_project_id varchar,
  p_trace_id varchar,
  p_actor_type varchar,
  p_actor_id varchar,

  p_task_type varchar,
  p_pipeline_version varchar,

  p_policy_version_id varchar,
  p_enabled boolean,

  p_input_contract_evidence_ref uuid,
  p_output_contract_evidence_ref uuid,

  p_default_mode varchar,
  p_idempotency_key text
)
RETURNS TABLE(contract_id bigint, change_kind varchar(16), found_existing boolean)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
BEGIN
  RETURN QUERY
  SELECT * FROM public.task_type_contract_upsert_v18(
    p_project_id,
    p_task_type,
    p_pipeline_version,
    p_policy_version_id,
    p_enabled,
    p_input_contract_evidence_ref,
    p_output_contract_evidence_ref,
    p_default_mode,
    p_actor_type,
    p_actor_id,
    p_trace_id,
    NULL::uuid,
    p_idempotency_key
  );
END$$;

-- =========================================================
-- enable / disable wrappers（8 args）
-- =========================================================
CREATE OR REPLACE FUNCTION public.task_type_contract_enable_v18(
  p_project_id       varchar,
  p_task_type        varchar,
  p_pipeline_version varchar,
  p_trace_id         varchar,
  p_created_by_type  varchar,
  p_created_by_id    varchar,
  p_run_id           uuid,
  p_idempotency_key  text
)
RETURNS TABLE(contract_id bigint, change_kind varchar(16), found_existing boolean)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  ex public.task_type_contracts%ROWTYPE;
BEGIN
  SELECT * INTO ex
  FROM public.task_type_contracts
  WHERE project_id=btrim(p_project_id)::varchar(26)
    AND task_type=btrim(p_task_type)::varchar(32)
    AND pipeline_version=btrim(p_pipeline_version)::varchar(64)
  FOR UPDATE;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'task_type_contract not found' USING ERRCODE='NO_DATA_FOUND';
  END IF;

  RETURN QUERY
  SELECT * FROM public.task_type_contract_upsert_v18(
    ex.project_id, ex.task_type, ex.pipeline_version,
    ex.policy_version_id,
    true,
    ex.input_contract_evidence_ref,
    ex.output_contract_evidence_ref,
    ex.default_mode,
    p_created_by_type, p_created_by_id,
    p_trace_id, p_run_id, p_idempotency_key
  );
END$$;

CREATE OR REPLACE FUNCTION public.task_type_contract_disable_v18(
  p_project_id       varchar,
  p_task_type        varchar,
  p_pipeline_version varchar,
  p_trace_id         varchar,
  p_created_by_type  varchar,
  p_created_by_id    varchar,
  p_run_id           uuid,
  p_idempotency_key  text
)
RETURNS TABLE(contract_id bigint, change_kind varchar(16), found_existing boolean)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  ex public.task_type_contracts%ROWTYPE;
BEGIN
  SELECT * INTO ex
  FROM public.task_type_contracts
  WHERE project_id=btrim(p_project_id)::varchar(26)
    AND task_type=btrim(p_task_type)::varchar(32)
    AND pipeline_version=btrim(p_pipeline_version)::varchar(64)
  FOR UPDATE;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'task_type_contract not found' USING ERRCODE='NO_DATA_FOUND';
  END IF;

  RETURN QUERY
  SELECT * FROM public.task_type_contract_upsert_v18(
    ex.project_id, ex.task_type, ex.pipeline_version,
    ex.policy_version_id,
    false,
    ex.input_contract_evidence_ref,
    ex.output_contract_evidence_ref,
    ex.default_mode,
    p_created_by_type, p_created_by_id,
    p_trace_id, p_run_id, p_idempotency_key
  );
END$$;

-- permissions
REVOKE ALL ON FUNCTION public.task_type_contract_upsert_v18(
  varchar, varchar, varchar, varchar, boolean, uuid, uuid, varchar, varchar, varchar, varchar, uuid, text
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.task_type_contract_upsert_v18(
  varchar, varchar, varchar, varchar,
  varchar, varchar,
  varchar, boolean,
  uuid, uuid,
  varchar, text
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.task_type_contract_enable_v18(
  varchar, varchar, varchar, varchar, varchar, varchar, uuid, text
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.task_type_contract_disable_v18(
  varchar, varchar, varchar, varchar, varchar, varchar, uuid, text
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.task_type_contract_upsert_v18(
  varchar, varchar, varchar, varchar, boolean, uuid, uuid, varchar, varchar, varchar, varchar, uuid, text
) TO ak;

GRANT EXECUTE ON FUNCTION public.task_type_contract_upsert_v18(
  varchar, varchar, varchar, varchar,
  varchar, varchar,
  varchar, boolean,
  uuid, uuid,
  varchar, text
) TO ak;

GRANT EXECUTE ON FUNCTION public.task_type_contract_enable_v18(
  varchar, varchar, varchar, varchar, varchar, varchar, uuid, text
) TO ak;

GRANT EXECUTE ON FUNCTION public.task_type_contract_disable_v18(
  varchar, varchar, varchar, varchar, varchar, varchar, uuid, text
) TO ak;

COMMIT;