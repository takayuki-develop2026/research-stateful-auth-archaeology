-- migrations/0057_v50_route_decisions.sql
-- v5.0: route_decisions (decision ledger)
-- Evidence alignment: use evidence_ref (uuid) as the canonical reference (v18)
-- Depends: projects, runs, providers, provider_routes, evidence_assets(project_id,evidence_ref)
BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.route_decisions (
  decision_id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id             varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  subject_type           varchar(32)  NOT NULL, -- payment_intent|catalog_publish|providerintel_run|...
  subject_internal_id    varchar(128) NOT NULL, -- v7主語

  subject_provider       varchar(64)  NULL,     -- optional (audit)
  subject_provider_object_type varchar(64) NULL,
  subject_provider_object_id   varchar(128) NULL,

  policy_version         varchar(32) NOT NULL,
  pipeline_version       varchar(32) NOT NULL,
  routing_version        varchar(16) NOT NULL DEFAULT 'v5',

  input_fingerprint      varchar(64) NOT NULL, -- sha256 stable_json
  chosen_route_id        uuid NULL REFERENCES public.provider_routes(route_id) ON DELETE RESTRICT,
  chosen_provider_id     uuid NULL REFERENCES public.providers(provider_id) ON DELETE RESTRICT,

  fallback_used          boolean NOT NULL DEFAULT false,

  status                 varchar(24) NOT NULL, -- chosen|review_required|denied
  denied_reason          varchar(64) NULL,

  why                    jsonb NOT NULL DEFAULT '{}'::jsonb, -- lightweight
  why_evidence_ref       uuid NOT NULL,                      -- heavy evidence reference (uuid)

  utl_commit_event_key   varchar(128) NOT NULL,              -- v6 internal event_key with namespace

  trace_id               varchar(128) NOT NULL,
  run_id                 uuid NOT NULL REFERENCES public.runs(run_id) ON DELETE RESTRICT,

  created_at             timestamptz NOT NULL DEFAULT now()
);

-- constraints
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='route_decisions_subject_nonempty') THEN
    ALTER TABLE public.route_decisions
      ADD CONSTRAINT route_decisions_subject_nonempty CHECK (btrim(subject_internal_id::text) <> '' AND btrim(subject_type::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='route_decisions_versions_nonempty') THEN
    ALTER TABLE public.route_decisions
      ADD CONSTRAINT route_decisions_versions_nonempty CHECK (btrim(policy_version::text) <> '' AND btrim(pipeline_version::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='route_decisions_fp_len') THEN
    ALTER TABLE public.route_decisions
      ADD CONSTRAINT route_decisions_fp_len CHECK (length(input_fingerprint::text) = 64);
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='route_decisions_status_ck') THEN
    ALTER TABLE public.route_decisions
      ADD CONSTRAINT route_decisions_status_ck CHECK (lower(status::text) IN ('chosen','review_required','denied'));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='route_decisions_trace_nonempty') THEN
    ALTER TABLE public.route_decisions
      ADD CONSTRAINT route_decisions_trace_nonempty CHECK (btrim(trace_id::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='route_decisions_utl_key_nonempty') THEN
    ALTER TABLE public.route_decisions
      ADD CONSTRAINT route_decisions_utl_key_nonempty CHECK (btrim(utl_commit_event_key::text) <> '' AND length(utl_commit_event_key::text) <= 128);
  END IF;

  -- enforce namespace (v6 contract)
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='route_decisions_utl_key_ns') THEN
    ALTER TABLE public.route_decisions
      ADD CONSTRAINT route_decisions_utl_key_ns CHECK (utl_commit_event_key LIKE 'utl_internal:%');
  END IF;

  -- idempotency-ish uniqueness per policy (your contract)
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_route_decisions_subject_policy') THEN
    ALTER TABLE public.route_decisions
      ADD CONSTRAINT ux_route_decisions_subject_policy UNIQUE (project_id, subject_type, subject_internal_id, policy_version);
  END IF;
END$$;

-- FK to evidence_assets by (project_id, evidence_ref)
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='route_decisions_why_evidence_fk') THEN
    ALTER TABLE public.route_decisions
      ADD CONSTRAINT route_decisions_why_evidence_fk
      FOREIGN KEY (project_id, why_evidence_ref)
      REFERENCES public.evidence_assets(project_id, evidence_ref)
      ON DELETE RESTRICT;
  END IF;
END$$;

-- indexes
CREATE INDEX IF NOT EXISTS idx_route_decisions_project_subject
  ON public.route_decisions(project_id, subject_type, subject_internal_id);

CREATE INDEX IF NOT EXISTS idx_route_decisions_project_trace
  ON public.route_decisions(project_id, trace_id);

CREATE INDEX IF NOT EXISTS idx_route_decisions_project_run
  ON public.route_decisions(project_id, run_id);

CREATE INDEX IF NOT EXISTS idx_route_decisions_project_chosen
  ON public.route_decisions(project_id, chosen_provider_id, chosen_route_id);

CREATE INDEX IF NOT EXISTS idx_route_decisions_project_utl_key
  ON public.route_decisions(project_id, utl_commit_event_key);

COMMIT;