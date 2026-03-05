BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.policy_decisions_v21 (
  id BIGSERIAL PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  decision_key char(64) NOT NULL CHECK (length(decision_key)=64),
  trace_id text NOT NULL,
  run_id uuid NULL REFERENCES public.runs(run_id) ON DELETE SET NULL,

  decision_time_utc timestamptz NOT NULL DEFAULT now(),

  subject_type text NOT NULL CHECK (subject_type IN ('user','service','api_key')),
  subject_id text NOT NULL,

  action_key text NOT NULL,
  action_class text NOT NULL CHECK (action_class IN ('high_risk','low_risk_read','low_risk_write')),

  policy_version_str text NOT NULL,
  result text NOT NULL CHECK (result IN ('allow','deny','error')),

  input_hash_sha256 char(64) NOT NULL CHECK (length(input_hash_sha256)=64),

  decision_input_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  decision_result_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  resource_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  obligations_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  reason_codes_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_at_utc timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT policy_decisions_v21_project_nonempty CHECK (btrim(project_id::text) <> ''),
  CONSTRAINT policy_decisions_v21_trace_nonempty CHECK (btrim(trace_id) <> ''),
  CONSTRAINT policy_decisions_v21_action_nonempty CHECK (btrim(action_key) <> ''),
  CONSTRAINT policy_decisions_v21_subject_nonempty CHECK (btrim(subject_id) <> ''),
  CONSTRAINT policy_decisions_v21_policy_ver_nonempty CHECK (btrim(policy_version_str) <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_policy_decisions_v21_project_key
  ON public.policy_decisions_v21(project_id, decision_key);

CREATE INDEX IF NOT EXISTS idx_policy_decisions_v21_project_time
  ON public.policy_decisions_v21(project_id, decision_time_utc DESC);

CREATE INDEX IF NOT EXISTS idx_policy_decisions_v21_trace
  ON public.policy_decisions_v21(trace_id);

CREATE INDEX IF NOT EXISTS idx_policy_decisions_v21_project_action_time
  ON public.policy_decisions_v21(project_id, action_key, decision_time_utc DESC);

COMMIT;