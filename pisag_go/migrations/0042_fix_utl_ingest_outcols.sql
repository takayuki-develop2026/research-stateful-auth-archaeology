BEGIN;

DROP FUNCTION IF EXISTS public.utl_ingest_v6(
  varchar, varchar, varchar, varchar, varchar,
  timestamptz, timestamptz,
  varchar, int,
  uuid, uuid,
  bigint, char, varchar, varchar, varchar,
  bigint
);

CREATE OR REPLACE FUNCTION public.utl_ingest_v6(
  p_project_id varchar,
  p_event_source varchar,
  p_provider varchar,
  p_provider_event_id varchar,
  p_event_name varchar,

  p_event_time timestamptz,
  p_received_at timestamptz,

  p_correlation_id varchar,
  p_event_seq int,

  p_trace_id uuid,
  p_run_id uuid,

  p_amount_minor bigint,
  p_currency char(3),
  p_region varchar,
  p_internal_object_id varchar,
  p_provider_object_id varchar,

  p_payload_evidence_asset_id bigint
)
RETURNS TABLE (
  out_utl_event_id bigint,
  out_status varchar(16),
  out_event_key varchar(128),
  out_posting_key char(64)
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_source text := lower(btrim(coalesce(p_event_source::text,'')));
  v_provider text := btrim(coalesce(p_provider::text,''));
  v_event_name text := btrim(coalesce(p_event_name::text,''));
  v_provider_event_id text := btrim(coalesce(p_provider_event_id::text,''));
  v_corr text := btrim(coalesce(p_correlation_id::text,''));

  v_event_key text;
  v_posting_key text;

  v_event_time timestamptz;
  v_received_at timestamptz;

  v_id bigint;
  v_row_status text;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_source NOT IN ('webhook','internal') THEN RAISE EXCEPTION 'event_source invalid' USING ERRCODE='22023'; END IF;
  IF v_provider='' THEN RAISE EXCEPTION 'provider required' USING ERRCODE='22023'; END IF;
  IF v_event_name='' THEN RAISE EXCEPTION 'event_name required' USING ERRCODE='22023'; END IF;
  IF p_trace_id IS NULL THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects WHERE id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  v_event_time := COALESCE(p_event_time, now());
  v_received_at := COALESCE(p_received_at, now());

  IF v_source='internal' THEN
    IF v_corr='' OR p_event_seq IS NULL THEN
      v_event_key := 'utl_internal:' || left(public.sha256_hex('internal|'||v_project_id||'|'||coalesce(nullif(v_corr,''),'MISSING_CORRELATION')||'|'||v_provider||'|'||v_event_name||'|'||coalesce(p_event_seq::text,'MISSING_SEQ')), 64);
      v_posting_key := left(public.sha256_hex(v_event_key), 64);
    ELSE
      v_event_key := 'utl_internal:' || left(public.sha256_hex('internal|'||v_project_id||'|'||v_corr||'|'||v_provider||'|'||v_event_name||'|'||p_event_seq::text), 64);
      v_posting_key := left(public.sha256_hex(public.posting_base(v_event_time, p_amount_minor, p_currency, coalesce(p_provider_object_id,''), v_event_key)), 64);
    END IF;
  ELSE
    IF v_provider_event_id='' THEN
      v_event_key := 'webhook:' || left(public.sha256_hex('webhook|'||v_project_id||'|'||v_provider||'|MISSING_PROVIDER_EVENT|'||v_event_name), 64);
      v_posting_key := left(public.sha256_hex(v_event_key), 64);
    ELSE
      v_event_key := 'webhook:' || left(public.sha256_hex('webhook|'||v_project_id||'|'||v_provider||'|'||v_provider_event_id||'|'||v_event_name), 64);
      v_posting_key := left(public.sha256_hex(public.posting_base(v_event_time, p_amount_minor, p_currency, coalesce(p_provider_object_id,''), v_event_key)), 64);
    END IF;
  END IF;

  SELECT e.id INTO v_id
  FROM public.universal_events_v6 e
  WHERE e.project_id=v_project_id::varchar(26) AND e.event_key=v_event_key::varchar(128)
  LIMIT 1;

  IF v_id IS NOT NULL THEN
    out_utl_event_id := v_id;
    out_status := 'duplicate';
    out_event_key := v_event_key::varchar(128);
    out_posting_key := v_posting_key::char(64);
    RETURN NEXT;
    RETURN;
  END IF;

  IF p_payload_evidence_asset_id IS NOT NULL THEN
    PERFORM 1 FROM public.evidence_assets WHERE id=p_payload_evidence_asset_id AND project_id=v_project_id::varchar(26);
    IF NOT FOUND THEN
      p_payload_evidence_asset_id := NULL;
    END IF;
  END IF;

  INSERT INTO public.universal_events_v6(
    project_id,
    event_source, provider, provider_event_id, event_name,
    event_key, posting_key,
    event_time, received_at,
    correlation_id, event_seq,
    trace_id, run_id,
    amount_minor, currency, region,
    internal_object_id, provider_object_id,
    status, process_attempts,
    payload_evidence_asset_id,
    created_at, updated_at
  )
  VALUES (
    v_project_id::varchar(26),
    v_source::varchar(16),
    v_provider::varchar(32),
    NULLIF(v_provider_event_id,'')::varchar(128),
    v_event_name::varchar(64),
    v_event_key::varchar(128),
    v_posting_key::char(64),
    v_event_time,
    v_received_at,
    NULLIF(v_corr,'')::varchar(128),
    p_event_seq,
    p_trace_id,
    p_run_id,
    p_amount_minor,
    p_currency,
    NULLIF(btrim(coalesce(p_region::text,'')),'')::varchar(16),
    NULLIF(btrim(coalesce(p_internal_object_id::text,'')),'')::varchar(128),
    NULLIF(btrim(coalesce(p_provider_object_id::text,'')),'')::varchar(128),
    CASE
      WHEN v_source='webhook' AND v_provider_event_id='' THEN 'review_required'
      WHEN v_source='internal' AND (v_corr='' OR p_event_seq IS NULL) THEN 'review_required'
      ELSE 'ingested'
    END::varchar(16),
    0,
    p_payload_evidence_asset_id,
    now(), now()
  )
  RETURNING id, status INTO v_id, v_row_status;

  out_utl_event_id := v_id;
  out_status := v_row_status::varchar(16);
  out_event_key := v_event_key::varchar(128);
  out_posting_key := v_posting_key::char(64);
  RETURN NEXT;
END;
$$;

-- keep permissions consistent: PUBLIC none, ak execute
REVOKE ALL ON FUNCTION public.utl_ingest_v6(
  varchar,varchar,varchar,varchar,varchar,timestamptz,timestamptz,varchar,int,uuid,uuid,bigint,char,varchar,varchar,varchar,bigint
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.utl_ingest_v6(
  varchar,varchar,varchar,varchar,varchar,timestamptz,timestamptz,varchar,int,uuid,uuid,bigint,char,varchar,varchar,varchar,bigint
) TO ak;

COMMIT;