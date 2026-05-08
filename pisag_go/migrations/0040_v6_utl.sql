-- migrations/0040_v6_utl.sql
-- v6 UTL (Universal Transaction Ledger) - minimal finance fact SoT
-- Aligns with current system:
-- - projects.id varchar(26)
-- - trace_id uuid
-- - evidence_assets.id bigint
-- - EXECUTE ONLY (SECURITY DEFINER) pattern
-- - No tenant_id in v6 P0 (can Expand later)

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================
-- 1) Tables
-- ============================================================

CREATE TABLE IF NOT EXISTS public.universal_events_v6 (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  event_source varchar(16) NOT NULL,          -- webhook|internal
  provider varchar(32) NOT NULL,              -- stripe|adyen|internal
  provider_event_id varchar(128) NULL,
  event_name varchar(64) NOT NULL,

  event_key varchar(128) NOT NULL UNIQUE,     -- namespace + hash
  posting_key char(64) NOT NULL,

  event_time timestamptz NOT NULL,
  received_at timestamptz NOT NULL,

  correlation_id varchar(128) NULL,
  event_seq int NULL,

  trace_id uuid NOT NULL,
  run_id uuid NULL,

  utx_id varchar(64) NULL,
  uop_id varchar(64) NULL,

  amount_minor bigint NULL,
  currency char(3) NULL,
  region varchar(16) NULL,

  internal_object_id varchar(128) NULL,
  provider_object_id varchar(128) NULL,

  status varchar(16) NOT NULL,                -- ingested|duplicate|processed|needs_retry|review_required
  process_attempts int NOT NULL DEFAULT 0,

  last_error_code varchar(64) NULL,
  last_error_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  payload_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT utl_event_source_ck CHECK (event_source IN ('webhook','internal')),
  CONSTRAINT utl_status_ck CHECK (status IN ('ingested','duplicate','processed','needs_retry','review_required')),
  CONSTRAINT utl_provider_nonempty CHECK (btrim(provider) <> ''),
  CONSTRAINT utl_event_name_nonempty CHECK (btrim(event_name) <> '')
);

CREATE INDEX IF NOT EXISTS idx_utl_v6_project_time
  ON public.universal_events_v6(project_id, received_at DESC);

CREATE INDEX IF NOT EXISTS idx_utl_v6_project_provider_event
  ON public.universal_events_v6(project_id, provider, provider_event_id);

CREATE INDEX IF NOT EXISTS idx_utl_v6_project_correlation_seq
  ON public.universal_events_v6(project_id, correlation_id, event_seq);


CREATE TABLE IF NOT EXISTS public.universal_event_evidence_links_v6 (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  utl_event_id bigint NOT NULL REFERENCES public.universal_events_v6(id) ON DELETE CASCADE,

  role varchar(32) NOT NULL, -- why|diff|normalized|verify|debug|...
  evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT ue_role_nonempty CHECK (btrim(role) <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_ue_ev_v6
  ON public.universal_event_evidence_links_v6(project_id, utl_event_id, role, evidence_asset_id);


CREATE TABLE IF NOT EXISTS public.utl_replay_requests_v6 (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  event_key varchar(128) NOT NULL,
  trace_id uuid NOT NULL,

  reason_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  status varchar(16) NOT NULL DEFAULT 'queued', -- queued|noop|review_required
  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT replay_status_ck CHECK (status IN ('queued','noop','review_required'))
);

CREATE INDEX IF NOT EXISTS idx_utl_replay_v6_project_time
  ON public.utl_replay_requests_v6(project_id, created_at DESC);


-- ============================================================
-- 2) Helpers: sha256 hex
-- ============================================================

CREATE OR REPLACE FUNCTION public.sha256_hex(p_text text)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$
  SELECT encode(digest(coalesce(p_text,''), 'sha256'), 'hex');
$$;


-- ============================================================
-- 3) EXECUTE ONLY functions
-- ============================================================

-- 3.1 ingest
-- Returns: utl_event_id, status, event_key, posting_key
CREATE OR REPLACE FUNCTION public.utl_ingest_v6(
  p_project_id varchar,
  p_event_source varchar,          -- webhook|internal
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
  utl_event_id bigint,
  status varchar(16),
  event_key varchar(128),
  posting_key char(64)
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

  v_exists bigint;
  v_event_time timestamptz;
  v_received_at timestamptz;
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

  -- Validate internal requirements (P0 strict to keep CI stable)
  IF v_source='internal' THEN
    IF v_corr='' THEN
      -- stateful: review_required (no throw)
      v_event_key := 'utl_internal:' || left(public.sha256_hex('internal|'||v_project_id||'|MISSING_CORRELATION|'||v_provider||'|'||v_event_name||'|0'), 64);
      v_posting_key := left(public.sha256_hex(v_event_key), 64);
    ELSE
      IF p_event_seq IS NULL THEN
        v_event_key := 'utl_internal:' || left(public.sha256_hex('internal|'||v_project_id||'|'||v_corr||'|'||v_provider||'|'||v_event_name||'|MISSING_SEQ'), 64);
        v_posting_key := left(public.sha256_hex(v_event_key), 64);
      ELSE
        v_event_key := 'utl_internal:' || left(public.sha256_hex('internal|'||v_project_id||'|'||v_corr||'|'||v_provider||'|'||v_event_name||'|'||p_event_seq::text), 64);
        v_posting_key := left(public.sha256_hex(posting_base(v_event_time, p_amount_minor, p_currency, coalesce(p_provider_object_id,''), v_event_key)), 64);
      END IF;
    END IF;
  ELSE
    -- webhook
    IF v_provider_event_id='' THEN
      v_event_key := 'webhook:' || left(public.sha256_hex('webhook|'||v_project_id||'|'||v_provider||'|MISSING_PROVIDER_EVENT|'||v_event_name), 64);
      v_posting_key := left(public.sha256_hex(v_event_key), 64);
    ELSE
      v_event_key := 'webhook:' || left(public.sha256_hex('webhook|'||v_project_id||'|'||v_provider||'|'||v_provider_event_id||'|'||v_event_name), 64);
      v_posting_key := left(public.sha256_hex(posting_base(v_event_time, p_amount_minor, p_currency, coalesce(p_provider_object_id,''), v_event_key)), 64);
    END IF;
  END IF;

  -- Dedup by UNIQUE(event_key)
  SELECT id INTO v_exists
  FROM public.universal_events_v6
  WHERE project_id=v_project_id::varchar(26) AND event_key=v_event_key::varchar(128)
  LIMIT 1;

  IF v_exists IS NOT NULL THEN
    utl_event_id := v_exists;
    status := 'duplicate';
    event_key := v_event_key::varchar(128);
    posting_key := v_posting_key::char(64);
    RETURN NEXT;
    RETURN;
  END IF;

  -- Optional evidence existence checks (do not throw into 500; but function runs in DB so we must decide)
  IF p_payload_evidence_asset_id IS NOT NULL THEN
    PERFORM 1 FROM public.evidence_assets WHERE id=p_payload_evidence_asset_id AND project_id=v_project_id::varchar(26);
    IF NOT FOUND THEN
      -- payload missing -> review_required, and clear payload link
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
    v_source::varchar(16), v_provider::varchar(32), NULLIF(v_provider_event_id,'')::varchar(128), v_event_name::varchar(64),
    v_event_key::varchar(128), v_posting_key::char(64),
    v_event_time, v_received_at,
    NULLIF(v_corr,'')::varchar(128), p_event_seq,
    p_trace_id, p_run_id,
    p_amount_minor, p_currency, NULLIF(btrim(coalesce(p_region::text,'')),'')::varchar(16),
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
  RETURNING id INTO v_exists;

  utl_event_id := v_exists;
  status := (SELECT status FROM public.universal_events_v6 WHERE id=v_exists);
  event_key := v_event_key::varchar(128);
  posting_key := v_posting_key::char(64);
  RETURN NEXT;
END;
$$;

-- Helper to compute posting base string (kept small)
CREATE OR REPLACE FUNCTION public.posting_base(
  p_event_time timestamptz,
  p_amount_minor bigint,
  p_currency char(3),
  p_provider_object_id text,
  p_event_key text
)
RETURNS text
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
  v_epoch text;
  v_amt text;
  v_ccy text;
  v_obj text;
BEGIN
  v_epoch := floor(extract(epoch from p_event_time))::bigint::text;

  IF p_amount_minor IS NULL OR p_currency IS NULL OR btrim(coalesce(p_currency::text,''))='' THEN
    RETURN p_event_key;
  END IF;

  v_amt := p_amount_minor::text;
  v_ccy := p_currency::text;

  v_obj := btrim(coalesce(p_provider_object_id,''));
  IF v_obj = '' THEN
    v_obj := p_event_key;
  END IF;

  RETURN v_epoch || '|' || v_amt || '|' || v_ccy || '|' || v_obj;
END;
$$;

-- 3.2 get by event_key
CREATE OR REPLACE FUNCTION public.utl_get_event_v6(
  p_project_id varchar,
  p_event_key varchar
)
RETURNS TABLE (
  id bigint,
  project_id varchar(26),
  event_source varchar(16),
  provider varchar(32),
  provider_event_id varchar(128),
  event_name varchar(64),
  event_key varchar(128),
  posting_key char(64),
  event_time timestamptz,
  received_at timestamptz,
  correlation_id varchar(128),
  event_seq int,
  trace_id uuid,
  run_id uuid,
  amount_minor bigint,
  currency char(3),
  region varchar(16),
  internal_object_id varchar(128),
  provider_object_id varchar(128),
  status varchar(16),
  process_attempts int,
  last_error_code varchar(64),
  last_error_evidence_asset_id bigint,
  payload_evidence_asset_id bigint,
  created_at timestamptz,
  updated_at timestamptz
)
LANGUAGE sql
SECURITY DEFINER
SET search_path = public
AS $$
  SELECT
    e.id, e.project_id, e.event_source, e.provider, e.provider_event_id, e.event_name,
    e.event_key, e.posting_key, e.event_time, e.received_at,
    e.correlation_id, e.event_seq, e.trace_id, e.run_id,
    e.amount_minor, e.currency, e.region, e.internal_object_id, e.provider_object_id,
    e.status, e.process_attempts,
    e.last_error_code, e.last_error_evidence_asset_id, e.payload_evidence_asset_id,
    e.created_at, e.updated_at
  FROM public.universal_events_v6 e
  WHERE e.project_id = p_project_id::varchar(26) AND e.event_key = p_event_key::varchar(128)
  LIMIT 1;
$$;

-- 3.3 request replay (v19 hook later)
CREATE OR REPLACE FUNCTION public.utl_request_replay_v6(
  p_project_id varchar,
  p_event_key varchar,
  p_trace_id uuid,
  p_reason_evidence_asset_id bigint
)
RETURNS TABLE(status varchar(16))
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_event_key text := btrim(coalesce(p_event_key::text,''));
  v_exists bigint;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_event_key='' THEN RAISE EXCEPTION 'event_key required' USING ERRCODE='22023'; END IF;
  IF p_trace_id IS NULL THEN RAISE EXCEPTION 'trace_id required' USING ERRCODE='22023'; END IF;

  SELECT id INTO v_exists
  FROM public.universal_events_v6
  WHERE project_id=v_project_id::varchar(26) AND event_key=v_event_key::varchar(128)
  LIMIT 1;

  IF v_exists IS NULL THEN
    status := 'review_required';
    RETURN NEXT;
    RETURN;
  END IF;

  IF p_reason_evidence_asset_id IS NOT NULL THEN
    PERFORM 1 FROM public.evidence_assets WHERE id=p_reason_evidence_asset_id AND project_id=v_project_id::varchar(26);
    IF NOT FOUND THEN
      p_reason_evidence_asset_id := NULL;
    END IF;
  END IF;

  INSERT INTO public.utl_replay_requests_v6(project_id, event_key, trace_id, reason_evidence_asset_id, status, created_at)
  VALUES (v_project_id::varchar(26), v_event_key::varchar(128), p_trace_id, p_reason_evidence_asset_id, 'queued', now());

  status := 'queued';
  RETURN NEXT;
END;
$$;


-- ============================================================
-- 4) Permissions (EXECUTE ONLY)
-- ============================================================

REVOKE ALL ON TABLE public.universal_events_v6 FROM PUBLIC;
REVOKE ALL ON TABLE public.universal_event_evidence_links_v6 FROM PUBLIC;
REVOKE ALL ON TABLE public.utl_replay_requests_v6 FROM PUBLIC;

REVOKE ALL ON FUNCTION public.utl_ingest_v6(
  varchar,varchar,varchar,varchar,varchar,timestamptz,timestamptz,varchar,int,uuid,uuid,bigint,char,varchar,varchar,varchar,bigint
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.utl_get_event_v6(varchar,varchar) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.utl_request_replay_v6(varchar,varchar,uuid,bigint) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.utl_ingest_v6(
  varchar,varchar,varchar,varchar,varchar,timestamptz,timestamptz,varchar,int,uuid,uuid,bigint,char,varchar,varchar,varchar,bigint
) TO ak;

GRANT EXECUTE ON FUNCTION public.utl_get_event_v6(varchar,varchar) TO ak;
GRANT EXECUTE ON FUNCTION public.utl_request_replay_v6(varchar,varchar,uuid,bigint) TO ak;

COMMIT;