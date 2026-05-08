-- migrations/0030_v18_audit_events_append_fn.sql
-- v18: audit_events append (EXECUTE ONLY) + idempotency (optional but recommended)
--
-- Existing audit_events schema (your DB):
--   project_id varchar(26) NOT NULL
--   trace_id   varchar(64) NOT NULL
--   run_id     varchar(26) NULL
--   action     varchar(64) NOT NULL
--   actor_type varchar(16) NOT NULL
--   actor_id   varchar(128) NULL
--   target_type varchar(64) NULL
--   target_id   varchar(128) NULL
--   result     varchar(16) NOT NULL  -- ok|denied|failed
--   reason     text NULL
--   meta       jsonb NULL
--   created_at timestamptz NOT NULL default now()
--
-- Goal:
-- - App/worker must NOT insert directly; call SECURITY DEFINER function only.

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ------------------------------------------------------------
-- optional idempotency table (recommended)
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.audit_idempotency_v18 (
  id              bigserial PRIMARY KEY,
  project_id      varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  scope           varchar(64) NOT NULL, -- fixed: 'audit_event_append_v18'
  idempotency_key text        NOT NULL,
  audit_event_id  bigint      NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT audit_idem_project_nonempty CHECK (btrim(project_id::text) <> ''),
  CONSTRAINT audit_idem_scope_nonempty CHECK (btrim(scope::text) <> ''),
  CONSTRAINT audit_idem_key_nonempty CHECK (btrim(idempotency_key::text) <> '')
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_audit_idem_v18') THEN
    ALTER TABLE public.audit_idempotency_v18
      ADD CONSTRAINT ux_audit_idem_v18 UNIQUE (project_id, scope, idempotency_key);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_audit_idem_v18_project_time
  ON public.audit_idempotency_v18(project_id, created_at DESC);

-- ------------------------------------------------------------
-- SECURITY DEFINER function
-- ------------------------------------------------------------
CREATE OR REPLACE FUNCTION public.audit_event_append_v18(
  p_project_id varchar,
  p_trace_id   varchar,
  p_run_id     varchar,   -- nullable (runs.run_id in your table is varchar(26))
  p_action     varchar,
  p_actor_type varchar,
  p_actor_id   varchar,   -- nullable
  p_target_type varchar,  -- nullable
  p_target_id   varchar,  -- nullable
  p_result     varchar,   -- ok|denied|failed
  p_reason     text,      -- nullable
  p_meta       jsonb,     -- nullable
  p_idempotency_key text  -- nullable but recommended
)
RETURNS TABLE (
  audit_event_id bigint,
  found_existing boolean
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_scope text := 'audit_event_append_v18';
  v_existing_id bigint;
  v_actor_type text;
  v_result text;
  v_project_id text;
  v_trace_id text;
  v_action text;
BEGIN
  v_project_id := btrim(coalesce(p_project_id::text, ''));
  IF v_project_id = '' THEN
    RAISE EXCEPTION 'project_id is required' USING ERRCODE='22023';
  END IF;

  v_trace_id := btrim(coalesce(p_trace_id::text, ''));
  IF v_trace_id = '' THEN
    RAISE EXCEPTION 'trace_id is required' USING ERRCODE='22023';
  END IF;

  v_action := btrim(coalesce(p_action::text, ''));
  IF v_action = '' THEN
    RAISE EXCEPTION 'action is required' USING ERRCODE='22023';
  END IF;

  v_actor_type := lower(btrim(coalesce(p_actor_type::text, '')));
  IF v_actor_type NOT IN ('system','user','service') THEN
    RAISE EXCEPTION 'actor_type must be system|user|service' USING ERRCODE='22023';
  END IF;

  v_result := lower(btrim(coalesce(p_result::text, '')));
  IF v_result NOT IN ('ok','denied','failed') THEN
    RAISE EXCEPTION 'result must be ok|denied|failed' USING ERRCODE='22023';
  END IF;

  -- Ensure project exists
  PERFORM 1 FROM public.projects WHERE id = v_project_id::varchar(26);
  IF NOT FOUND THEN
    RAISE EXCEPTION 'project not found: %', v_project_id USING ERRCODE='23503';
  END IF;

  -- Idempotency fast path
  IF NULLIF(btrim(coalesce(p_idempotency_key,'')), '') IS NOT NULL THEN
    SELECT a.audit_event_id INTO v_existing_id
    FROM public.audit_idempotency_v18 a
    WHERE a.project_id = v_project_id::varchar(26)
      AND a.scope = v_scope
      AND a.idempotency_key = p_idempotency_key
    LIMIT 1;

    IF v_existing_id IS NOT NULL THEN
      audit_event_id := v_existing_id;
      found_existing := true;
      RETURN NEXT;
      RETURN;
    END IF;
  END IF;

  -- Insert audit_events
  INSERT INTO public.audit_events(
    project_id, trace_id, run_id, action,
    actor_type, actor_id,
    target_type, target_id,
    result, reason, meta,
    created_at
  )
  VALUES (
    v_project_id::varchar(26),
    v_trace_id::varchar(64),
    NULLIF(btrim(coalesce(p_run_id::text,'')),'')::varchar(26),
    v_action::varchar(64),

    v_actor_type::varchar(16),
    NULLIF(btrim(coalesce(p_actor_id::text,'')),'')::varchar(128),

    NULLIF(btrim(coalesce(p_target_type::text,'')),'')::varchar(64),
    NULLIF(btrim(coalesce(p_target_id::text,'')),'')::varchar(128),

    v_result::varchar(16),
    p_reason,
    p_meta,
    now()
  )
  RETURNING id INTO v_existing_id;

  -- Record idempotency mapping (if key supplied)
  IF NULLIF(btrim(coalesce(p_idempotency_key,'')), '') IS NOT NULL THEN
    INSERT INTO public.audit_idempotency_v18(project_id, scope, idempotency_key, audit_event_id, created_at)
    VALUES (v_project_id::varchar(26), v_scope, p_idempotency_key, v_existing_id, now())
    ON CONFLICT (project_id, scope, idempotency_key) DO NOTHING;
  END IF;

  audit_event_id := v_existing_id;
  found_existing := false;
  RETURN NEXT;
END;
$$;

REVOKE ALL ON FUNCTION public.audit_event_append_v18(
  varchar, varchar, varchar, varchar, varchar, varchar, varchar, varchar, varchar, text, jsonb, text
) FROM PUBLIC;

-- 実行権限：まずは ak のみ（必要なら ak_worker / service role に追加）
GRANT EXECUTE ON FUNCTION public.audit_event_append_v18(
  varchar, varchar, varchar, varchar, varchar, varchar, varchar, varchar, varchar, text, jsonb, text
) TO ak;

COMMIT;