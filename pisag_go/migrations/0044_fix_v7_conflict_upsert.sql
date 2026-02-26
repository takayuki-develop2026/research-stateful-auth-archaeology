-- migrations/0044_fix_v7_conflict_upsert.sql
-- Fix: partial unique index ux_idc_v7_open is NOT a constraint, so ON CONFLICT ON CONSTRAINT fails.
-- Solution: explicit get-or-create open conflict helper + re-create resolve/assign using it.

BEGIN;

-- ------------------------------------------------------------
-- Helper: get or create an OPEN conflict (never throws upstream)
-- ------------------------------------------------------------
CREATE OR REPLACE FUNCTION public.v7_conflict_get_or_create_open(
  p_project_id varchar,
  p_provider varchar,
  p_provider_object_type varchar,
  p_provider_object_id varchar,
  p_candidate_internal_object_type varchar,
  p_candidate_internal_object_id varchar,
  p_conflict_type varchar,      -- duplicate_provider_id|multiple_active_targets|reverse_collision|type_mismatch
  p_event_key_ref varchar,
  p_trace_id uuid,
  p_payload_evidence_asset_id bigint
)
RETURNS bigint
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_provider text := btrim(coalesce(p_provider::text,''));
  v_ptype text := btrim(coalesce(p_provider_object_type::text,''));
  v_pid text := btrim(coalesce(p_provider_object_id::text,''));

  v_id bigint;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_provider='' THEN RAISE EXCEPTION 'provider required' USING ERRCODE='22023'; END IF;
  IF v_ptype='' THEN RAISE EXCEPTION 'provider_object_type required' USING ERRCODE='22023'; END IF;
  IF v_pid='' THEN RAISE EXCEPTION 'provider_object_id required' USING ERRCODE='22023'; END IF;
  IF p_trace_id IS NULL THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;

  -- reuse open if exists
  SELECT id INTO v_id
  FROM public.id_mapping_conflicts_v7
  WHERE project_id=v_project_id::varchar(26)
    AND provider=v_provider::varchar(32)
    AND provider_object_type=v_ptype::varchar(64)
    AND provider_object_id=v_pid::varchar(128)
    AND status='open'
  LIMIT 1;

  IF v_id IS NOT NULL THEN
    UPDATE public.id_mapping_conflicts_v7 SET updated_at=now() WHERE id=v_id;
    RETURN v_id;
  END IF;

  -- create new open
  BEGIN
    INSERT INTO public.id_mapping_conflicts_v7(
      project_id, provider, provider_object_type, provider_object_id,
      candidate_internal_object_type, candidate_internal_object_id,
      conflict_type, status, event_key_ref, trace_id, payload_evidence_asset_id,
      created_at, updated_at
    )
    VALUES (
      v_project_id::varchar(26), v_provider::varchar(32), v_ptype::varchar(64), v_pid::varchar(128),
      NULLIF(btrim(coalesce(p_candidate_internal_object_type::text,'')),'')::varchar(64),
      NULLIF(btrim(coalesce(p_candidate_internal_object_id::text,'')),'')::varchar(128),
      btrim(coalesce(p_conflict_type::text,''))::varchar(32),
      'open',
      NULLIF(btrim(coalesce(p_event_key_ref::text,'')),'')::varchar(128),
      p_trace_id,
      p_payload_evidence_asset_id,
      now(), now()
    )
    RETURNING id INTO v_id;

    RETURN v_id;

  EXCEPTION WHEN unique_violation THEN
    -- another concurrent insert created the open row; reuse it
    SELECT id INTO v_id
    FROM public.id_mapping_conflicts_v7
    WHERE project_id=v_project_id::varchar(26)
      AND provider=v_provider::varchar(32)
      AND provider_object_type=v_ptype::varchar(64)
      AND provider_object_id=v_pid::varchar(128)
      AND status='open'
    LIMIT 1;

    IF v_id IS NULL THEN
      -- should be extremely rare; fail closed into a new open row without relying on partial index
      INSERT INTO public.id_mapping_conflicts_v7(
        project_id, provider, provider_object_type, provider_object_id,
        candidate_internal_object_type, candidate_internal_object_id,
        conflict_type, status, event_key_ref, trace_id, payload_evidence_asset_id,
        created_at, updated_at
      )
      VALUES (
        v_project_id::varchar(26), v_provider::varchar(32), v_ptype::varchar(64), v_pid::varchar(128),
        NULLIF(btrim(coalesce(p_candidate_internal_object_type::text,'')),'')::varchar(64),
        NULLIF(btrim(coalesce(p_candidate_internal_object_id::text,'')),'')::varchar(128),
        'multiple_active_targets','open',
        NULLIF(btrim(coalesce(p_event_key_ref::text,'')),'')::varchar(128),
        p_trace_id,
        p_payload_evidence_asset_id,
        now(), now()
      )
      RETURNING id INTO v_id;
    END IF;

    RETURN v_id;
  END;
END;
$$;

REVOKE ALL ON FUNCTION public.v7_conflict_get_or_create_open(
  varchar, varchar, varchar, varchar, varchar, varchar, varchar, varchar, uuid, bigint
) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.v7_conflict_get_or_create_open(
  varchar, varchar, varchar, varchar, varchar, varchar, varchar, varchar, uuid, bigint
) TO ak;

-- ------------------------------------------------------------
-- Re-create identity_resolve_v7 (fixed conflict creation)
-- ------------------------------------------------------------
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
    UPDATE public.id_mappings_v7 SET last_seen_at=now(), updated_at=now() WHERE id=v_existing_id;

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

  -- create requires valid internal_object_type
  IF v_itype='' OR v_itype NOT IN ('payment','utx','uop') THEN
    v_conflict_id := public.v7_conflict_get_or_create_open(
      v_project_id::varchar(26),
      v_provider::varchar(32),
      v_ptype::varchar(64),
      v_pid::varchar(128),
      COALESCE(NULLIF(v_itype,''),'(missing)')::varchar(64),
      '(none)'::varchar(128),
      'type_mismatch',
      p_event_key_ref,
      p_trace_id,
      p_payload_evidence_asset_id
    );

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

    out_status := 'created';
    out_internal_object_type := v_itype::varchar(64);
    out_internal_object_id := v_new_internal_id::varchar(128);
    out_mapping_id := v_existing_id;
    out_conflict_id := NULL;
    RETURN NEXT; RETURN;

  EXCEPTION WHEN unique_violation THEN
    -- Another writer inserted active. Re-select and return resolved
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
    v_conflict_id := public.v7_conflict_get_or_create_open(
      v_project_id::varchar(26),
      v_provider::varchar(32),
      v_ptype::varchar(64),
      v_pid::varchar(128),
      v_itype::varchar(64),
      v_new_internal_id::varchar(128),
      'multiple_active_targets',
      p_event_key_ref,
      p_trace_id,
      p_payload_evidence_asset_id
    );

    out_status := 'review_required';
    out_internal_object_type := NULL;
    out_internal_object_id := NULL;
    out_mapping_id := NULL;
    out_conflict_id := v_conflict_id;
    RETURN NEXT; RETURN;
  END;
END;
$$;

-- ------------------------------------------------------------
-- Re-create identity_assign_v7 (fixed conflict creation)
-- ------------------------------------------------------------
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
      v_conflict_id := public.v7_conflict_get_or_create_open(
        v_project_id::varchar(26),
        v_provider::varchar(32),
        v_ptype::varchar(64),
        v_pid::varchar(128),
        v_itype::varchar(64),
        v_iid::varchar(128),
        'duplicate_provider_id',
        p_event_key_ref,
        p_trace_id,
        p_payload_evidence_asset_id
      );

      out_status := 'review_required';
      out_mapping_id := NULL;
      out_conflict_id := v_conflict_id;
      RETURN NEXT; RETURN;
    END IF;

    -- supersede_active
    UPDATE public.id_mappings_v7 SET mapping_status='superseded', updated_at=now() WHERE id=v_active_id;
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

COMMIT;