-- migrations/0043_v7_identity.sql
-- v7 Dual Tokenization (provider id <-> internal id) - P0
-- internal_object_type allowed: payment|utx|uop
-- Evidence: use evidence_assets(id bigint) + link tables (NO json arrays)
-- trace_id: uuid
-- EXECUTE ONLY: SECURITY DEFINER functions

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================
-- 1) Tables
-- ============================================================

-- 1.1 id_mappings_v7 (SoT)
CREATE TABLE IF NOT EXISTS public.id_mappings_v7 (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  provider varchar(32) NOT NULL,
  provider_object_type varchar(64) NOT NULL,
  provider_object_id varchar(128) NOT NULL,

  internal_object_type varchar(64) NOT NULL, -- payment|utx|uop
  internal_object_id varchar(128) NOT NULL,  -- pay_/utx_/uop_ + token

  mapping_status varchar(16) NOT NULL,       -- active|superseded|conflict|review_required
  source varchar(16) NOT NULL,               -- webhook|internal|reconcile|manual
  reason_code varchar(64) NULL,

  first_seen_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),

  event_key_ref varchar(128) NULL,           -- v6 universal_events_v6.event_key (stored ref)
  trace_id uuid NOT NULL,

  payload_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT idm_provider_nonempty CHECK (btrim(provider) <> ''),
  CONSTRAINT idm_provider_type_nonempty CHECK (btrim(provider_object_type) <> ''),
  CONSTRAINT idm_provider_id_nonempty CHECK (btrim(provider_object_id) <> ''),
  CONSTRAINT idm_internal_type_ck CHECK (internal_object_type IN ('payment','utx','uop')),
  CONSTRAINT idm_status_ck CHECK (mapping_status IN ('active','superseded','conflict','review_required')),
  CONSTRAINT idm_source_ck CHECK (source IN ('webhook','internal','reconcile','manual'))
);

CREATE INDEX IF NOT EXISTS idx_idm_v7_provider_lookup
  ON public.id_mappings_v7(project_id, provider, provider_object_type, provider_object_id);

CREATE INDEX IF NOT EXISTS idx_idm_v7_internal_lookup
  ON public.id_mappings_v7(project_id, internal_object_type, internal_object_id);

CREATE INDEX IF NOT EXISTS idx_idm_v7_status_lookup
  ON public.id_mappings_v7(project_id, mapping_status, provider, provider_object_type);

-- Active uniqueness (prevents race-induced double actives).
-- We will CATCH unique_violation in functions and convert to conflict (no upstream crash).
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_class WHERE relname='ux_idm_v7_active') THEN
    CREATE UNIQUE INDEX ux_idm_v7_active
      ON public.id_mappings_v7(project_id, provider, provider_object_type, provider_object_id)
      WHERE mapping_status = 'active';
  END IF;
END$$;

-- 1.2 id_mapping_conflicts_v7
CREATE TABLE IF NOT EXISTS public.id_mapping_conflicts_v7 (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  provider varchar(32) NOT NULL,
  provider_object_type varchar(64) NOT NULL,
  provider_object_id varchar(128) NOT NULL,

  candidate_internal_object_type varchar(64) NOT NULL,
  candidate_internal_object_id varchar(128) NOT NULL,

  conflict_type varchar(32) NOT NULL, -- duplicate_provider_id|multiple_active_targets|reverse_collision|type_mismatch
  status varchar(16) NOT NULL DEFAULT 'open', -- open|resolved|ignored

  event_key_ref varchar(128) NULL,
  trace_id uuid NOT NULL,

  payload_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT idc_status_ck CHECK (status IN ('open','resolved','ignored')),
  CONSTRAINT idc_conflict_type_ck CHECK (conflict_type IN ('duplicate_provider_id','multiple_active_targets','reverse_collision','type_mismatch'))
);

CREATE INDEX IF NOT EXISTS idx_idc_v7_project_status
  ON public.id_mapping_conflicts_v7(project_id, status, conflict_type, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_idc_v7_provider_lookup
  ON public.id_mapping_conflicts_v7(project_id, provider, provider_object_type, provider_object_id);

-- One open conflict per provider object (reuse)
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_class WHERE relname='ux_idc_v7_open') THEN
    CREATE UNIQUE INDEX ux_idc_v7_open
      ON public.id_mapping_conflicts_v7(project_id, provider, provider_object_type, provider_object_id)
      WHERE status = 'open';
  END IF;
END$$;

-- 1.3 evidence links (E clause)
CREATE TABLE IF NOT EXISTS public.id_mapping_evidence_links_v7 (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,
  mapping_id bigint NOT NULL REFERENCES public.id_mappings_v7(id) ON DELETE CASCADE,
  role varchar(32) NOT NULL,
  evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT idmel_role_ck CHECK (btrim(role) <> '')
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_idmel_v7
  ON public.id_mapping_evidence_links_v7(project_id, mapping_id, role, evidence_asset_id);

CREATE TABLE IF NOT EXISTS public.id_mapping_conflict_evidence_links_v7 (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,
  conflict_id bigint NOT NULL REFERENCES public.id_mapping_conflicts_v7(id) ON DELETE CASCADE,
  role varchar(32) NOT NULL,
  evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT idcel_role_ck CHECK (btrim(role) <> '')
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_idcel_v7
  ON public.id_mapping_conflict_evidence_links_v7(project_id, conflict_id, role, evidence_asset_id);

-- ============================================================
-- 2) Helpers
-- ============================================================

CREATE OR REPLACE FUNCTION public.v7_internal_id(prefix text)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$
  SELECT prefix || '_' || replace(gen_random_uuid()::text, '-', '');
$$;

CREATE OR REPLACE FUNCTION public.v7_prefix_for_type(p_type text)
RETURNS text
LANGUAGE plpgsql
IMMUTABLE
AS $$
BEGIN
  IF p_type='payment' THEN RETURN 'pay'; END IF;
  IF p_type='utx' THEN RETURN 'utx'; END IF;
  IF p_type='uop' THEN RETURN 'uop'; END IF;
  RETURN 'obj';
END;
$$;

-- ============================================================
-- 3) EXECUTE ONLY functions
-- ============================================================

-- 3.1 resolve (resolved|created|not_found|review_required)
CREATE OR REPLACE FUNCTION public.identity_resolve_v7(
  p_project_id varchar,
  p_provider varchar,
  p_provider_object_type varchar,
  p_provider_object_id varchar,

  p_internal_object_type varchar,   -- required if create_if_missing=true
  p_create_if_missing boolean,

  p_trace_id uuid,
  p_event_key_ref varchar,
  p_payload_evidence_asset_id bigint
)
RETURNS TABLE (
  out_status varchar(16),
  out_internal_object_type varchar(64),
  out_internal_object_id varchar(128),
  out_mapping_id bigint,
  out_conflict_id bigint
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_provider text := btrim(coalesce(p_provider::text,''));
  v_ptype text := btrim(coalesce(p_provider_object_type::text,''));
  v_pid text := btrim(coalesce(p_provider_object_id::text,''));
  v_itype text := btrim(coalesce(p_internal_object_type::text,''));

  v_existing_id bigint;
  v_existing_internal_type text;
  v_existing_internal_id text;

  v_new_internal_id text;
  v_conflict_id bigint;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_provider='' THEN RAISE EXCEPTION 'provider required' USING ERRCODE='22023'; END IF;
  IF v_ptype='' THEN RAISE EXCEPTION 'provider_object_type required' USING ERRCODE='22023'; END IF;
  IF v_pid='' THEN RAISE EXCEPTION 'provider_object_id required' USING ERRCODE='22023'; END IF;
  IF p_trace_id IS NULL THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects WHERE id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  IF p_payload_evidence_asset_id IS NOT NULL THEN
    PERFORM 1 FROM public.evidence_assets WHERE id=p_payload_evidence_asset_id AND project_id=v_project_id::varchar(26);
    IF NOT FOUND THEN
      p_payload_evidence_asset_id := NULL;
    END IF;
  END IF;

  -- 1) active exists -> resolved
  SELECT id, internal_object_type, internal_object_id
    INTO v_existing_id, v_existing_internal_type, v_existing_internal_id
  FROM public.id_mappings_v7
  WHERE project_id=v_project_id::varchar(26)
    AND provider=v_provider::varchar(32)
    AND provider_object_type=v_ptype::varchar(64)
    AND provider_object_id=v_pid::varchar(128)
    AND mapping_status='active'
  LIMIT 1;

  IF v_existing_id IS NOT NULL THEN
    UPDATE public.id_mappings_v7
      SET last_seen_at=now(), updated_at=now()
    WHERE id=v_existing_id;

    out_status := 'resolved';
    out_internal_object_type := v_existing_internal_type::varchar(64);
    out_internal_object_id := v_existing_internal_id::varchar(128);
    out_mapping_id := v_existing_id;
    out_conflict_id := NULL;
    RETURN NEXT; RETURN;
  END IF;

  -- 2) not found
  IF NOT COALESCE(p_create_if_missing, false) THEN
    out_status := 'not_found';
    out_internal_object_type := NULL;
    out_internal_object_id := NULL;
    out_mapping_id := NULL;
    out_conflict_id := NULL;
    RETURN NEXT; RETURN;
  END IF;

  -- create requires internal_object_type
  IF v_itype='' OR v_itype NOT IN ('payment','utx','uop') THEN
    -- type mismatch -> conflict (review_required)
    INSERT INTO public.id_mapping_conflicts_v7(
      project_id, provider, provider_object_type, provider_object_id,
      candidate_internal_object_type, candidate_internal_object_id,
      conflict_type, status, event_key_ref, trace_id, payload_evidence_asset_id,
      created_at, updated_at
    )
    VALUES (
      v_project_id::varchar(26), v_provider::varchar(32), v_ptype::varchar(64), v_pid::varchar(128),
      COALESCE(NULLIF(v_itype,''),'(missing)')::varchar(64), '(none)'::varchar(128),
      'type_mismatch','open', NULLIF(btrim(coalesce(p_event_key_ref::text,'')),'')::varchar(128), p_trace_id, p_payload_evidence_asset_id,
      now(), now()
    )
    ON CONFLICT ON CONSTRAINT ux_idc_v7_open DO UPDATE
      SET updated_at=now()
    RETURNING id INTO v_conflict_id;

    out_status := 'review_required';
    out_internal_object_type := NULL;
    out_internal_object_id := NULL;
    out_mapping_id := NULL;
    out_conflict_id := v_conflict_id;
    RETURN NEXT; RETURN;
  END IF;

  -- 3) create new mapping (race safe)
  v_new_internal_id := public.v7_internal_id(public.v7_prefix_for_type(v_itype));

  BEGIN
    INSERT INTO public.id_mappings_v7(
      project_id,
      provider, provider_object_type, provider_object_id,
      internal_object_type, internal_object_id,
      mapping_status, source, reason_code,
      first_seen_at, last_seen_at,
      event_key_ref, trace_id, payload_evidence_asset_id,
      created_at, updated_at
    )
    VALUES (
      v_project_id::varchar(26),
      v_provider::varchar(32), v_ptype::varchar(64), v_pid::varchar(128),
      v_itype::varchar(64), v_new_internal_id::varchar(128),
      'active','internal','first_seen',
      now(), now(),
      NULLIF(btrim(coalesce(p_event_key_ref::text,'')),'')::varchar(128),
      p_trace_id,
      p_payload_evidence_asset_id,
      now(), now()
    )
    RETURNING id INTO v_existing_id;

  EXCEPTION WHEN unique_violation THEN
    -- Another writer inserted active. Re-select and return resolved.
    SELECT id, internal_object_type, internal_object_id
      INTO v_existing_id, v_existing_internal_type, v_existing_internal_id
    FROM public.id_mappings_v7
    WHERE project_id=v_project_id::varchar(26)
      AND provider=v_provider::varchar(32)
      AND provider_object_type=v_ptype::varchar(64)
      AND provider_object_id=v_pid::varchar(128)
      AND mapping_status='active'
    LIMIT 1;

    IF v_existing_id IS NOT NULL THEN
      out_status := 'resolved';
      out_internal_object_type := v_existing_internal_type::varchar(64);
      out_internal_object_id := v_existing_internal_id::varchar(128);
      out_mapping_id := v_existing_id;
      out_conflict_id := NULL;
      RETURN NEXT; RETURN;
    END IF;

    -- If still not found, isolate conflict
    INSERT INTO public.id_mapping_conflicts_v7(
      project_id, provider, provider_object_type, provider_object_id,
      candidate_internal_object_type, candidate_internal_object_id,
      conflict_type, status, event_key_ref, trace_id, payload_evidence_asset_id,
      created_at, updated_at
    )
    VALUES (
      v_project_id::varchar(26), v_provider::varchar(32), v_ptype::varchar(64), v_pid::varchar(128),
      v_itype::varchar(64), v_new_internal_id::varchar(128),
      'multiple_active_targets','open', NULLIF(btrim(coalesce(p_event_key_ref::text,'')),'')::varchar(128), p_trace_id, p_payload_evidence_asset_id,
      now(), now()
    )
    ON CONFLICT ON CONSTRAINT ux_idc_v7_open DO UPDATE SET updated_at=now()
    RETURNING id INTO v_conflict_id;

    out_status := 'review_required';
    out_internal_object_type := NULL;
    out_internal_object_id := NULL;
    out_mapping_id := NULL;
    out_conflict_id := v_conflict_id;
    RETURN NEXT; RETURN;
  END;

  out_status := 'created';
  out_internal_object_type := v_itype::varchar(64);
  out_internal_object_id := v_new_internal_id::varchar(128);
  out_mapping_id := v_existing_id;
  out_conflict_id := NULL;
  RETURN NEXT;
END;
$$;

-- 3.2 assign (manual)
CREATE OR REPLACE FUNCTION public.identity_assign_v7(
  p_project_id varchar,
  p_provider varchar,
  p_provider_object_type varchar,
  p_provider_object_id varchar,

  p_internal_object_type varchar,
  p_internal_object_id varchar,

  p_mode varchar, -- set_active|supersede_active
  p_trace_id uuid,
  p_event_key_ref varchar,
  p_payload_evidence_asset_id bigint
)
RETURNS TABLE (
  out_status varchar(16),   -- assigned|review_required
  out_mapping_id bigint,
  out_conflict_id bigint
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_provider text := btrim(coalesce(p_provider::text,''));
  v_ptype text := btrim(coalesce(p_provider_object_type::text,''));
  v_pid text := btrim(coalesce(p_provider_object_id::text,''));

  v_itype text := btrim(coalesce(p_internal_object_type::text,''));
  v_iid text := btrim(coalesce(p_internal_object_id::text,''));

  v_mode text := lower(btrim(coalesce(p_mode::text,'')));

  v_active_id bigint;
  v_active_iid text;
  v_conflict_id bigint;
  v_new_id bigint;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_provider='' THEN RAISE EXCEPTION 'provider required' USING ERRCODE='22023'; END IF;
  IF v_ptype='' THEN RAISE EXCEPTION 'provider_object_type required' USING ERRCODE='22023'; END IF;
  IF v_pid='' THEN RAISE EXCEPTION 'provider_object_id required' USING ERRCODE='22023'; END IF;
  IF v_itype NOT IN ('payment','utx','uop') THEN RAISE EXCEPTION 'internal_object_type invalid' USING ERRCODE='22023'; END IF;
  IF v_iid='' THEN RAISE EXCEPTION 'internal_object_id required' USING ERRCODE='22023'; END IF;
  IF v_mode NOT IN ('set_active','supersede_active') THEN RAISE EXCEPTION 'mode invalid' USING ERRCODE='22023'; END IF;
  IF p_trace_id IS NULL THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects WHERE id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  IF p_payload_evidence_asset_id IS NOT NULL THEN
    PERFORM 1 FROM public.evidence_assets WHERE id=p_payload_evidence_asset_id AND project_id=v_project_id::varchar(26);
    IF NOT FOUND THEN
      p_payload_evidence_asset_id := NULL;
    END IF;
  END IF;

  -- lock current active row if exists
  SELECT id, internal_object_id INTO v_active_id, v_active_iid
  FROM public.id_mappings_v7
  WHERE project_id=v_project_id::varchar(26)
    AND provider=v_provider::varchar(32)
    AND provider_object_type=v_ptype::varchar(64)
    AND provider_object_id=v_pid::varchar(128)
    AND mapping_status='active'
  FOR UPDATE
  LIMIT 1;

  IF v_active_id IS NOT NULL THEN
    IF v_active_iid = v_iid THEN
      out_status := 'assigned';
      out_mapping_id := v_active_id;
      out_conflict_id := NULL;
      RETURN NEXT; RETURN;
    END IF;

    IF v_mode='set_active' THEN
      INSERT INTO public.id_mapping_conflicts_v7(
        project_id, provider, provider_object_type, provider_object_id,
        candidate_internal_object_type, candidate_internal_object_id,
        conflict_type, status, event_key_ref, trace_id, payload_evidence_asset_id,
        created_at, updated_at
      )
      VALUES (
        v_project_id::varchar(26), v_provider::varchar(32), v_ptype::varchar(64), v_pid::varchar(128),
        v_itype::varchar(64), v_iid::varchar(128),
        'duplicate_provider_id','open',
        NULLIF(btrim(coalesce(p_event_key_ref::text,'')),'')::varchar(128),
        p_trace_id,
        p_payload_evidence_asset_id,
        now(), now()
      )
      ON CONFLICT ON CONSTRAINT ux_idc_v7_open DO UPDATE SET updated_at=now()
      RETURNING id INTO v_conflict_id;

      out_status := 'review_required';
      out_mapping_id := NULL;
      out_conflict_id := v_conflict_id;
      RETURN NEXT; RETURN;
    END IF;

    -- supersede_active
    UPDATE public.id_mappings_v7
      SET mapping_status='superseded', updated_at=now()
    WHERE id=v_active_id;
  END IF;

  -- create new active mapping
  INSERT INTO public.id_mappings_v7(
    project_id, provider, provider_object_type, provider_object_id,
    internal_object_type, internal_object_id,
    mapping_status, source, reason_code,
    first_seen_at, last_seen_at,
    event_key_ref, trace_id, payload_evidence_asset_id,
    created_at, updated_at
  )
  VALUES (
    v_project_id::varchar(26), v_provider::varchar(32), v_ptype::varchar(64), v_pid::varchar(128),
    v_itype::varchar(64), v_iid::varchar(128),
    'active','manual','manual_assign',
    now(), now(),
    NULLIF(btrim(coalesce(p_event_key_ref::text,'')),'')::varchar(128),
    p_trace_id, p_payload_evidence_asset_id,
    now(), now()
  )
  RETURNING id INTO v_new_id;

  out_status := 'assigned';
  out_mapping_id := v_new_id;
  out_conflict_id := NULL;
  RETURN NEXT;
END;
$$;

-- ============================================================
-- 4) Permissions (EXECUTE ONLY)
-- ============================================================

REVOKE ALL ON TABLE public.id_mappings_v7 FROM PUBLIC;
REVOKE ALL ON TABLE public.id_mapping_conflicts_v7 FROM PUBLIC;
REVOKE ALL ON TABLE public.id_mapping_evidence_links_v7 FROM PUBLIC;
REVOKE ALL ON TABLE public.id_mapping_conflict_evidence_links_v7 FROM PUBLIC;

REVOKE ALL ON FUNCTION public.identity_resolve_v7(
  varchar,varchar,varchar,varchar,varchar,boolean,uuid,varchar,bigint
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.identity_assign_v7(
  varchar,varchar,varchar,varchar,varchar,varchar,varchar,uuid,varchar,bigint
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.identity_resolve_v7(
  varchar,varchar,varchar,varchar,varchar,boolean,uuid,varchar,bigint
) TO ak;

GRANT EXECUTE ON FUNCTION public.identity_assign_v7(
  varchar,varchar,varchar,varchar,varchar,varchar,varchar,uuid,varchar,bigint
) TO ak;

COMMIT;