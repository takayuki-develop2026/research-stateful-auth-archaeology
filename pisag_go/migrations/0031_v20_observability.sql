-- migrations/0031_v20_observability.sql
-- v20: Observability / SLO / Incidents / Proposals / Remediation (EXECUTE ONLY)
-- Integrates:
--   - v18 projects (projects.id = varchar(26))
--   - v4 runs (runs.run_id = uuid, trace_id = uuid)
--   - evidence_assets (id = bigint)
-- E clause: SoT tables hold only keys/status/numerics + evidence_asset_id pointers (NO body/json/text payload).
-- Mutations MUST go through SECURITY DEFINER functions.

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================
-- 1) Tables
-- ============================================================

-- 1.1 telemetry_span_summaries
CREATE TABLE IF NOT EXISTS public.telemetry_span_summaries (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  trace_id uuid NOT NULL,
  span_key text NOT NULL, -- sha256(trace_id + service + operation + started_at_utc)

  run_id uuid NULL REFERENCES public.runs(run_id) ON DELETE SET NULL,

  service varchar(64) NOT NULL,
  operation varchar(128) NOT NULL,
  status varchar(16) NOT NULL, -- ok|error

  started_at_utc timestamptz NOT NULL,
  ended_at_utc timestamptz NULL,

  summary_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT span_status_valid CHECK (lower(status) IN ('ok','error')),
  CONSTRAINT span_key_nonempty CHECK (btrim(span_key) <> ''),
  CONSTRAINT span_service_nonempty CHECK (btrim(service) <> ''),
  CONSTRAINT span_operation_nonempty CHECK (btrim(operation) <> '')
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_span_summary_v20') THEN
    ALTER TABLE public.telemetry_span_summaries
      ADD CONSTRAINT ux_span_summary_v20 UNIQUE (project_id, span_key);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_span_summaries_v20_project_time
  ON public.telemetry_span_summaries(project_id, started_at_utc DESC);

CREATE INDEX IF NOT EXISTS idx_span_summaries_v20_trace
  ON public.telemetry_span_summaries(trace_id);

CREATE INDEX IF NOT EXISTS idx_span_summaries_v20_run
  ON public.telemetry_span_summaries(run_id);


-- 1.2 telemetry_metric_rollups
CREATE TABLE IF NOT EXISTS public.telemetry_metric_rollups (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  metric_key varchar(128) NOT NULL,
  time_bucket varchar(16) NOT NULL, -- minute|hour|day
  bucket_start_at_utc timestamptz NOT NULL,

  value numeric NOT NULL,

  dimensions_key text NOT NULL, -- sha256(canonical_dimensions_text)
  dimensions_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT bucket_valid CHECK (lower(time_bucket) IN ('minute','hour','day')),
  CONSTRAINT metric_key_nonempty CHECK (btrim(metric_key) <> ''),
  CONSTRAINT dimensions_key_nonempty CHECK (btrim(dimensions_key) <> '')
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_metric_rollup_v20') THEN
    ALTER TABLE public.telemetry_metric_rollups
      ADD CONSTRAINT ux_metric_rollup_v20 UNIQUE (project_id, metric_key, time_bucket, bucket_start_at_utc, dimensions_key);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_metric_rollups_v20_project_time
  ON public.telemetry_metric_rollups(project_id, bucket_start_at_utc DESC);


-- 1.3 slo_definitions
CREATE TABLE IF NOT EXISTS public.slo_definitions (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  name varchar(128) NOT NULL,
  enabled boolean NOT NULL DEFAULT false,
  window_kind varchar(16) NOT NULL, -- 7d|30d
  target numeric NOT NULL,

  slo_spec_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  severity_policy_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  alert_policy_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_by_type varchar(16) NOT NULL, -- system|user|service
  created_by_id varchar(128) NULL,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT window_kind_valid CHECK (lower(window_kind) IN ('7d','30d')),
  CONSTRAINT created_by_type_valid CHECK (lower(created_by_type) IN ('system','user','service')),
  CONSTRAINT slo_name_nonempty CHECK (btrim(name) <> '')
);

CREATE INDEX IF NOT EXISTS idx_slo_definitions_v20_project_enabled
  ON public.slo_definitions(project_id, enabled);


-- 1.4 slo_evaluations
CREATE TABLE IF NOT EXISTS public.slo_evaluations (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  slo_id bigint NOT NULL REFERENCES public.slo_definitions(id) ON DELETE CASCADE,

  evaluation_key text NOT NULL,
  window_start_at_utc timestamptz NOT NULL,
  window_end_at_utc timestamptz NOT NULL,

  sli_value numeric NOT NULL,
  error_budget_remaining numeric NOT NULL,

  status varchar(16) NOT NULL, -- ok|warn|breach
  evaluated_at_utc timestamptz NOT NULL DEFAULT now(),

  evaluation_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  CONSTRAINT eval_status_valid CHECK (lower(status) IN ('ok','warn','breach')),
  CONSTRAINT evaluation_key_nonempty CHECK (btrim(evaluation_key) <> '')
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_slo_eval_v20') THEN
    ALTER TABLE public.slo_evaluations
      ADD CONSTRAINT ux_slo_eval_v20 UNIQUE (project_id, evaluation_key);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_slo_evaluations_v20_project_time
  ON public.slo_evaluations(project_id, evaluated_at_utc DESC);

CREATE INDEX IF NOT EXISTS idx_slo_evaluations_v20_slo_time
  ON public.slo_evaluations(slo_id, evaluated_at_utc DESC);


-- 1.5 incidents
CREATE TABLE IF NOT EXISTS public.incidents (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  incident_key text NOT NULL,

  status varchar(16) NOT NULL, -- open|triaging|mitigating|resolved|closed
  severity varchar(8) NOT NULL, -- P1|P2|P3|P4
  incident_type varchar(64) NOT NULL,

  root_trace_id uuid NULL,
  root_run_id uuid NULL REFERENCES public.runs(run_id) ON DELETE SET NULL,

  detected_by varchar(16) NOT NULL, -- slo|rule|manual
  detected_at_utc timestamptz NOT NULL DEFAULT now(),

  incident_summary_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  primary_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  owner_user_id varchar(128) NULL,
  resolved_at_utc timestamptz NULL,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT incident_key_nonempty CHECK (btrim(incident_key) <> ''),
  CONSTRAINT incident_status_valid CHECK (lower(status) IN ('open','triaging','mitigating','resolved','closed')),
  CONSTRAINT incident_severity_valid CHECK (upper(severity) IN ('P1','P2','P3','P4')),
  CONSTRAINT detected_by_valid CHECK (lower(detected_by) IN ('slo','rule','manual')),
  CONSTRAINT incident_type_nonempty CHECK (btrim(incident_type) <> '')
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_incident_v20') THEN
    ALTER TABLE public.incidents
      ADD CONSTRAINT ux_incident_v20 UNIQUE (project_id, incident_key);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_incidents_v20_project_status_time
  ON public.incidents(project_id, status, detected_at_utc DESC);

CREATE INDEX IF NOT EXISTS idx_incidents_v20_project_severity_time
  ON public.incidents(project_id, severity, detected_at_utc DESC);


-- 1.5.1 incident_labels
CREATE TABLE IF NOT EXISTS public.incident_labels (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  incident_id bigint NOT NULL REFERENCES public.incidents(id) ON DELETE CASCADE,

  label_key varchar(64) NOT NULL,
  label_value varchar(128) NOT NULL,

  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT label_key_nonempty CHECK (btrim(label_key) <> ''),
  CONSTRAINT label_value_nonempty CHECK (btrim(label_value) <> '')
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_incident_label_v20') THEN
    ALTER TABLE public.incident_labels
      ADD CONSTRAINT ux_incident_label_v20 UNIQUE (project_id, incident_id, label_key, label_value);
  END IF;
END$$;


-- 1.6 incident_events
CREATE TABLE IF NOT EXISTS public.incident_events (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  incident_id bigint NOT NULL REFERENCES public.incidents(id) ON DELETE CASCADE,

  event_type varchar(64) NOT NULL,
  event_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_by_type varchar(16) NOT NULL, -- system|user|service
  created_by_id varchar(128) NULL,

  created_at_utc timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT event_type_nonempty CHECK (btrim(event_type) <> ''),
  CONSTRAINT created_by_type_valid2 CHECK (lower(created_by_type) IN ('system','user','service'))
);

CREATE INDEX IF NOT EXISTS idx_incident_events_v20_project_incident_time
  ON public.incident_events(project_id, incident_id, created_at_utc DESC);


-- 1.7 remediation_proposals
CREATE TABLE IF NOT EXISTS public.remediation_proposals (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  incident_id bigint NOT NULL REFERENCES public.incidents(id) ON DELETE CASCADE,

  proposal_key text NOT NULL,
  proposal_type varchar(64) NOT NULL,

  status varchar(16) NOT NULL, -- proposed|needs_review|approved|rejected|applied|expired
  risk_level varchar(16) NOT NULL, -- low|medium|high
  requires_approval boolean NOT NULL DEFAULT true,

  proposal_plan_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  proposal_impact_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  proposal_primary_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  approved_by_user_id varchar(128) NULL,
  approved_at_utc timestamptz NULL,

  applied_by_user_id varchar(128) NULL,
  applied_at_utc timestamptz NULL,

  expires_at_utc timestamptz NULL,

  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT proposal_key_nonempty CHECK (btrim(proposal_key) <> ''),
  CONSTRAINT proposal_status_valid CHECK (lower(status) IN ('proposed','needs_review','approved','rejected','applied','expired')),
  CONSTRAINT risk_level_valid CHECK (lower(risk_level) IN ('low','medium','high')),
  CONSTRAINT proposal_type_nonempty CHECK (btrim(proposal_type) <> '')
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_proposal_v20') THEN
    ALTER TABLE public.remediation_proposals
      ADD CONSTRAINT ux_proposal_v20 UNIQUE (project_id, proposal_key);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_proposals_v20_project_status
  ON public.remediation_proposals(project_id, status);

CREATE INDEX IF NOT EXISTS idx_proposals_v20_project_incident_status
  ON public.remediation_proposals(project_id, incident_id, status);


-- 1.8 remediation_actions
CREATE TABLE IF NOT EXISTS public.remediation_actions (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  proposal_id bigint NOT NULL REFERENCES public.remediation_proposals(id) ON DELETE CASCADE,

  action_key text NOT NULL,
  run_id uuid NOT NULL REFERENCES public.runs(run_id) ON DELETE RESTRICT,

  status varchar(16) NOT NULL, -- queued|running|succeeded|failed
  action_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_at_utc timestamptz NOT NULL DEFAULT now(),
  updated_at_utc timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT action_key_nonempty CHECK (btrim(action_key) <> ''),
  CONSTRAINT action_status_valid CHECK (lower(status) IN ('queued','running','succeeded','failed'))
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_action_v20') THEN
    ALTER TABLE public.remediation_actions
      ADD CONSTRAINT ux_action_v20 UNIQUE (project_id, action_key);
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_actions_v20_project_proposal_time
  ON public.remediation_actions(project_id, proposal_id, created_at_utc DESC);


-- ============================================================
-- 2) Optional idempotency (recommended)
-- ============================================================

CREATE TABLE IF NOT EXISTS public.v20_idempotency (
  id bigserial PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,
  scope varchar(64) NOT NULL,
  idempotency_key text NOT NULL,
  entity_type varchar(64) NOT NULL,
  entity_id bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_v20_idem') THEN
    ALTER TABLE public.v20_idempotency
      ADD CONSTRAINT ux_v20_idem UNIQUE (project_id, scope, idempotency_key);
  END IF;
END$$;


-- ============================================================
-- 3) SECURITY DEFINER functions (EXECUTE ONLY)
-- ============================================================

-- 3.1 span_summary_upsert_v20
CREATE OR REPLACE FUNCTION public.span_summary_upsert_v20(
  p_project_id varchar,
  p_trace_id uuid,
  p_span_key text,
  p_run_id uuid,
  p_service varchar,
  p_operation varchar,
  p_status varchar,
  p_started_at_utc timestamptz,
  p_ended_at_utc timestamptz,
  p_summary_evidence_asset_id bigint
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_span_key text := btrim(coalesce(p_span_key::text,''));
  v_status text := lower(btrim(coalesce(p_status::text,'')));
BEGIN
  IF v_project_id = '' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_span_key = '' THEN RAISE EXCEPTION 'span_key required' USING ERRCODE='22023'; END IF;
  IF v_status NOT IN ('ok','error') THEN RAISE EXCEPTION 'status must be ok|error' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects WHERE id = v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found: %', v_project_id USING ERRCODE='23503'; END IF;

  PERFORM 1 FROM public.evidence_assets WHERE id = p_summary_evidence_asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'evidence_asset not found: %', p_summary_evidence_asset_id USING ERRCODE='23503'; END IF;

  INSERT INTO public.telemetry_span_summaries(
    project_id, trace_id, span_key, run_id, service, operation, status,
    started_at_utc, ended_at_utc, summary_evidence_asset_id, created_at
  )
  VALUES (
    v_project_id::varchar(26),
    p_trace_id,
    v_span_key,
    p_run_id,
    btrim(p_service)::varchar(64),
    btrim(p_operation)::varchar(128),
    v_status::varchar(16),
    p_started_at_utc,
    p_ended_at_utc,
    p_summary_evidence_asset_id,
    now()
  )
  ON CONFLICT (project_id, span_key) DO UPDATE
    SET ended_at_utc = EXCLUDED.ended_at_utc,
        status = EXCLUDED.status,
        summary_evidence_asset_id = EXCLUDED.summary_evidence_asset_id;
END;
$$;


-- 3.2 metric_rollup_upsert_v20
CREATE OR REPLACE FUNCTION public.metric_rollup_upsert_v20(
  p_project_id varchar,
  p_metric_key varchar,
  p_time_bucket varchar,
  p_bucket_start_at_utc timestamptz,
  p_value numeric,
  p_dimensions_key text,
  p_dimensions_evidence_asset_id bigint
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_bucket text := lower(btrim(coalesce(p_time_bucket::text,'')));
  v_metric_key text := btrim(coalesce(p_metric_key::text,''));
  v_dim_key text := btrim(coalesce(p_dimensions_key::text,''));
BEGIN
  IF v_project_id = '' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_metric_key = '' THEN RAISE EXCEPTION 'metric_key required' USING ERRCODE='22023'; END IF;
  IF v_bucket NOT IN ('minute','hour','day') THEN RAISE EXCEPTION 'time_bucket invalid' USING ERRCODE='22023'; END IF;
  IF v_dim_key = '' THEN RAISE EXCEPTION 'dimensions_key required' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects WHERE id = v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found: %', v_project_id USING ERRCODE='23503'; END IF;

  PERFORM 1 FROM public.evidence_assets WHERE id = p_dimensions_evidence_asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'evidence_asset not found: %', p_dimensions_evidence_asset_id USING ERRCODE='23503'; END IF;

  INSERT INTO public.telemetry_metric_rollups(
    project_id, metric_key, time_bucket, bucket_start_at_utc, value,
    dimensions_key, dimensions_evidence_asset_id, created_at
  )
  VALUES (
    v_project_id::varchar(26),
    v_metric_key::varchar(128),
    v_bucket::varchar(16),
    p_bucket_start_at_utc,
    p_value,
    v_dim_key,
    p_dimensions_evidence_asset_id,
    now()
  )
  ON CONFLICT (project_id, metric_key, time_bucket, bucket_start_at_utc, dimensions_key)
  DO UPDATE SET value = EXCLUDED.value, dimensions_evidence_asset_id = EXCLUDED.dimensions_evidence_asset_id;
END;
$$;


-- 3.3 incident_create_v20
CREATE OR REPLACE FUNCTION public.incident_create_v20(
  p_project_id varchar,
  p_incident_key text,
  p_status varchar,
  p_severity varchar,
  p_incident_type varchar,
  p_root_trace_id uuid,
  p_root_run_id uuid,
  p_detected_by varchar,
  p_detected_at_utc timestamptz,
  p_incident_summary_evidence_asset_id bigint,
  p_primary_evidence_asset_id bigint,
  p_owner_user_id varchar,
  p_idempotency_key text
)
RETURNS TABLE (incident_id bigint, found_existing boolean)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_scope text := 'incident_create_v20';
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_key text := btrim(coalesce(p_incident_key::text,''));
  v_status text := lower(btrim(coalesce(p_status::text,'')));
  v_sev text := upper(btrim(coalesce(p_severity::text,'')));
  v_detected_by text := lower(btrim(coalesce(p_detected_by::text,'')));
  v_existing bigint;
BEGIN
  IF v_project_id = '' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_key = '' THEN RAISE EXCEPTION 'incident_key required' USING ERRCODE='22023'; END IF;
  IF v_status NOT IN ('open','triaging','mitigating','resolved','closed') THEN RAISE EXCEPTION 'invalid status' USING ERRCODE='22023'; END IF;
  IF v_sev NOT IN ('P1','P2','P3','P4') THEN RAISE EXCEPTION 'invalid severity' USING ERRCODE='22023'; END IF;
  IF v_detected_by NOT IN ('slo','rule','manual') THEN RAISE EXCEPTION 'invalid detected_by' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects WHERE id = v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found: %', v_project_id USING ERRCODE='23503'; END IF;

  PERFORM 1 FROM public.evidence_assets WHERE id = p_incident_summary_evidence_asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'evidence_asset not found: %', p_incident_summary_evidence_asset_id USING ERRCODE='23503'; END IF;

  IF p_root_run_id IS NOT NULL THEN
    PERFORM 1 FROM public.runs WHERE run_id = p_root_run_id;
    IF NOT FOUND THEN RAISE EXCEPTION 'run not found: %', p_root_run_id USING ERRCODE='23503'; END IF;
  END IF;

  IF NULLIF(btrim(coalesce(p_idempotency_key,'')), '') IS NOT NULL THEN
    SELECT entity_id INTO v_existing
    FROM public.v20_idempotency
    WHERE project_id = v_project_id::varchar(26) AND scope=v_scope AND idempotency_key=p_idempotency_key
    LIMIT 1;
    IF v_existing IS NOT NULL THEN
      incident_id := v_existing;
      found_existing := true;
      RETURN NEXT; RETURN;
    END IF;
  END IF;

  INSERT INTO public.incidents(
    project_id, incident_key, status, severity, incident_type,
    root_trace_id, root_run_id,
    detected_by, detected_at_utc,
    incident_summary_evidence_asset_id, primary_evidence_asset_id,
    owner_user_id,
    created_at, updated_at
  )
  VALUES (
    v_project_id::varchar(26),
    v_key,
    v_status::varchar(16),
    v_sev::varchar(8),
    btrim(p_incident_type)::varchar(64),
    p_root_trace_id,
    p_root_run_id,
    v_detected_by::varchar(16),
    COALESCE(p_detected_at_utc, now()),
    p_incident_summary_evidence_asset_id,
    NULLIF(p_primary_evidence_asset_id, 0),
    NULLIF(btrim(coalesce(p_owner_user_id::text,'')),'')::varchar(128),
    now(), now()
  )
  ON CONFLICT (project_id, incident_key) DO UPDATE
    SET updated_at=now()
  RETURNING id INTO v_existing;

  IF NULLIF(btrim(coalesce(p_idempotency_key,'')), '') IS NOT NULL THEN
    INSERT INTO public.v20_idempotency(project_id, scope, idempotency_key, entity_type, entity_id, created_at)
    VALUES (v_project_id::varchar(26), v_scope, p_idempotency_key, 'incident', v_existing, now())
    ON CONFLICT (project_id, scope, idempotency_key) DO NOTHING;
  END IF;

  incident_id := v_existing;
  found_existing := false;
  RETURN NEXT;
END;
$$;


-- 3.4 proposal_create_v20 (forces needs_review)
CREATE OR REPLACE FUNCTION public.proposal_create_v20(
  p_project_id varchar,
  p_incident_id bigint,
  p_proposal_key text,
  p_proposal_type varchar,
  p_risk_level varchar,
  p_requires_approval boolean,
  p_plan_evidence_asset_id bigint,
  p_impact_evidence_asset_id bigint,
  p_primary_evidence_asset_id bigint,
  p_idempotency_key text
)
RETURNS TABLE (proposal_id bigint, found_existing boolean)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_scope text := 'proposal_create_v20';
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_key text := btrim(coalesce(p_proposal_key::text,''));
  v_risk text := lower(btrim(coalesce(p_risk_level::text,'')));
  v_existing bigint;
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_key='' THEN RAISE EXCEPTION 'proposal_key required' USING ERRCODE='22023'; END IF;
  IF v_risk NOT IN ('low','medium','high') THEN RAISE EXCEPTION 'risk_level invalid' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects WHERE id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found: %', v_project_id USING ERRCODE='23503'; END IF;

  PERFORM 1 FROM public.incidents WHERE id=p_incident_id AND project_id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'incident not found' USING ERRCODE='23503'; END IF;

  PERFORM 1 FROM public.evidence_assets WHERE id=p_plan_evidence_asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'plan evidence not found' USING ERRCODE='23503'; END IF;

  PERFORM 1 FROM public.evidence_assets WHERE id=p_impact_evidence_asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'impact evidence not found' USING ERRCODE='23503'; END IF;

  IF NULLIF(btrim(coalesce(p_idempotency_key,'')), '') IS NOT NULL THEN
    SELECT entity_id INTO v_existing
    FROM public.v20_idempotency
    WHERE project_id=v_project_id::varchar(26) AND scope=v_scope AND idempotency_key=p_idempotency_key
    LIMIT 1;
    IF v_existing IS NOT NULL THEN
      proposal_id := v_existing;
      found_existing := true;
      RETURN NEXT; RETURN;
    END IF;
  END IF;

  INSERT INTO public.remediation_proposals(
    project_id, incident_id, proposal_key, proposal_type,
    status, risk_level, requires_approval,
    proposal_plan_evidence_asset_id, proposal_impact_evidence_asset_id, proposal_primary_evidence_asset_id,
    created_at, updated_at
  )
  VALUES (
    v_project_id::varchar(26),
    p_incident_id,
    v_key,
    btrim(p_proposal_type)::varchar(64),
    'needs_review',
    v_risk::varchar(16),
    COALESCE(p_requires_approval, true),
    p_plan_evidence_asset_id,
    p_impact_evidence_asset_id,
    NULLIF(p_primary_evidence_asset_id, 0),
    now(), now()
  )
  ON CONFLICT (project_id, proposal_key) DO UPDATE
    SET updated_at=now()
  RETURNING id INTO v_existing;

  IF NULLIF(btrim(coalesce(p_idempotency_key,'')), '') IS NOT NULL THEN
    INSERT INTO public.v20_idempotency(project_id, scope, idempotency_key, entity_type, entity_id, created_at)
    VALUES (v_project_id::varchar(26), v_scope, p_idempotency_key, 'proposal', v_existing, now())
    ON CONFLICT (project_id, scope, idempotency_key) DO NOTHING;
  END IF;

  proposal_id := v_existing;
  found_existing := false;
  RETURN NEXT;
END;
$$;


-- 3.5 proposal_mark_approved_v20
CREATE OR REPLACE FUNCTION public.proposal_mark_approved_v20(
  p_project_id varchar,
  p_proposal_id bigint,
  p_approved_by_user_id varchar,
  p_approved_at_utc timestamptz
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;

  UPDATE public.remediation_proposals
  SET status='approved',
      approved_by_user_id=NULLIF(btrim(coalesce(p_approved_by_user_id::text,'')),'')::varchar(128),
      approved_at_utc=COALESCE(p_approved_at_utc, now()),
      updated_at=now()
  WHERE id=p_proposal_id AND project_id=v_project_id::varchar(26) AND status='needs_review';

  IF NOT FOUND THEN
    RAISE EXCEPTION 'proposal not found or not needs_review' USING ERRCODE='22023';
  END IF;
END;
$$;


-- 3.6 action_create_v20
CREATE OR REPLACE FUNCTION public.action_create_v20(
  p_project_id varchar,
  p_proposal_id bigint,
  p_action_key text,
  p_run_id uuid,
  p_status varchar,
  p_action_evidence_asset_id bigint
)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
DECLARE
  v_project_id text := btrim(coalesce(p_project_id::text,''));
  v_key text := btrim(coalesce(p_action_key::text,''));
  v_status text := lower(btrim(coalesce(p_status::text,'')));
BEGIN
  IF v_project_id='' THEN RAISE EXCEPTION 'project_id required' USING ERRCODE='22023'; END IF;
  IF v_key='' THEN RAISE EXCEPTION 'action_key required' USING ERRCODE='22023'; END IF;
  IF v_status NOT IN ('queued','running','succeeded','failed') THEN RAISE EXCEPTION 'invalid status' USING ERRCODE='22023'; END IF;

  PERFORM 1 FROM public.projects WHERE id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'project not found' USING ERRCODE='23503'; END IF;

  PERFORM 1 FROM public.remediation_proposals WHERE id=p_proposal_id AND project_id=v_project_id::varchar(26);
  IF NOT FOUND THEN RAISE EXCEPTION 'proposal not found' USING ERRCODE='23503'; END IF;

  PERFORM 1 FROM public.evidence_assets WHERE id=p_action_evidence_asset_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'evidence_asset not found: %', p_action_evidence_asset_id USING ERRCODE='23503'; END IF;

  PERFORM 1 FROM public.runs WHERE run_id = p_run_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'run not found: %', p_run_id USING ERRCODE='23503'; END IF;

  INSERT INTO public.remediation_actions(
    project_id, proposal_id, action_key, run_id, status, action_evidence_asset_id,
    created_at_utc, updated_at_utc
  )
  VALUES (
    v_project_id::varchar(26), p_proposal_id, v_key,
    p_run_id, v_status::varchar(16), p_action_evidence_asset_id,
    now(), now()
  )
  ON CONFLICT (project_id, action_key) DO UPDATE
    SET status=EXCLUDED.status,
        action_evidence_asset_id=EXCLUDED.action_evidence_asset_id,
        updated_at_utc=now();
END;
$$;


-- ============================================================
-- 4) Permissions (EXECUTE ONLY)
-- ============================================================

REVOKE ALL ON TABLE public.telemetry_span_summaries FROM PUBLIC;
REVOKE ALL ON TABLE public.telemetry_metric_rollups FROM PUBLIC;
REVOKE ALL ON TABLE public.slo_definitions FROM PUBLIC;
REVOKE ALL ON TABLE public.slo_evaluations FROM PUBLIC;
REVOKE ALL ON TABLE public.incidents FROM PUBLIC;
REVOKE ALL ON TABLE public.incident_labels FROM PUBLIC;
REVOKE ALL ON TABLE public.incident_events FROM PUBLIC;
REVOKE ALL ON TABLE public.remediation_proposals FROM PUBLIC;
REVOKE ALL ON TABLE public.remediation_actions FROM PUBLIC;
REVOKE ALL ON TABLE public.v20_idempotency FROM PUBLIC;

REVOKE ALL ON FUNCTION public.span_summary_upsert_v20(
  varchar, uuid, text, uuid, varchar, varchar, varchar, timestamptz, timestamptz, bigint
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.metric_rollup_upsert_v20(
  varchar, varchar, varchar, timestamptz, numeric, text, bigint
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.incident_create_v20(
  varchar, text, varchar, varchar, varchar, uuid, uuid, varchar, timestamptz, bigint, bigint, varchar, text
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.proposal_create_v20(
  varchar, bigint, text, varchar, varchar, boolean, bigint, bigint, bigint, text
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.proposal_mark_approved_v20(
  varchar, bigint, varchar, timestamptz
) FROM PUBLIC;

REVOKE ALL ON FUNCTION public.action_create_v20(
  varchar, bigint, text, uuid, varchar, bigint
) FROM PUBLIC;

-- worker emits spans/metrics
GRANT EXECUTE ON FUNCTION public.span_summary_upsert_v20(
  varchar, uuid, text, uuid, varchar, varchar, varchar, timestamptz, timestamptz, bigint
) TO ak_worker;

GRANT EXECUTE ON FUNCTION public.metric_rollup_upsert_v20(
  varchar, varchar, varchar, timestamptz, numeric, text, bigint
) TO ak_worker;

-- API/usecase role creates incident/proposal/approve/apply
GRANT EXECUTE ON FUNCTION public.incident_create_v20(
  varchar, text, varchar, varchar, varchar, uuid, uuid, varchar, timestamptz, bigint, bigint, varchar, text
) TO ak;

GRANT EXECUTE ON FUNCTION public.proposal_create_v20(
  varchar, bigint, text, varchar, varchar, boolean, bigint, bigint, bigint, text
) TO ak;

GRANT EXECUTE ON FUNCTION public.proposal_mark_approved_v20(
  varchar, bigint, varchar, timestamptz
) TO ak;

GRANT EXECUTE ON FUNCTION public.action_create_v20(
  varchar, bigint, text, uuid, varchar, bigint
) TO ak;

COMMIT;