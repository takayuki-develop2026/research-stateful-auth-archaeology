BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.key_rotation_plans_v21 (
  id BIGSERIAL PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  rotation_key char(64) NOT NULL CHECK (length(rotation_key)=64),
  key_domain text NOT NULL,
  status text NOT NULL CHECK (status IN ('planned','in_progress','verified','completed','aborted')),

  plan_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  verification_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_by_user_id text NOT NULL,
  created_at_utc timestamptz NOT NULL DEFAULT now(),
  completed_at_utc timestamptz NULL,

  CONSTRAINT kr_v21_project_nonempty CHECK (btrim(project_id::text) <> ''),
  CONSTRAINT kr_v21_domain_nonempty CHECK (btrim(key_domain) <> ''),
  CONSTRAINT kr_v21_created_by_nonempty CHECK (btrim(created_by_user_id) <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_key_rotation_plans_v21_project_key
  ON public.key_rotation_plans_v21(project_id, rotation_key);

CREATE INDEX IF NOT EXISTS idx_key_rotation_plans_v21_project_status_time
  ON public.key_rotation_plans_v21(project_id, status, created_at_utc DESC);

COMMIT;