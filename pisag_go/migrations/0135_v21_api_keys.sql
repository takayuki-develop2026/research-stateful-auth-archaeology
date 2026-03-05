BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.api_keys_v21 (
  id BIGSERIAL PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  key_id varchar(64) NOT NULL,
  key_hash char(64) NOT NULL CHECK (length(key_hash)=64),

  scope_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  status text NOT NULL CHECK (status IN ('active','revoked','expired')),
  expires_at_utc timestamptz NOT NULL,

  created_by_type varchar(16) NOT NULL CHECK (created_by_type IN ('system','user','service')),
  created_by_id varchar(128) NULL,

  created_at_utc timestamptz NOT NULL DEFAULT now(),
  revoked_at_utc timestamptz NULL,
  revoked_reason_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  CONSTRAINT api_keys_v21_key_id_nonempty CHECK (btrim(key_id::text) <> ''),
  CONSTRAINT api_keys_v21_project_nonempty CHECK (btrim(project_id::text) <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_api_keys_v21_project_keyid
  ON public.api_keys_v21(project_id, key_id);

CREATE INDEX IF NOT EXISTS idx_api_keys_v21_project_status
  ON public.api_keys_v21(project_id, status);

CREATE INDEX IF NOT EXISTS idx_api_keys_v21_project_expires
  ON public.api_keys_v21(project_id, expires_at_utc);

COMMIT;