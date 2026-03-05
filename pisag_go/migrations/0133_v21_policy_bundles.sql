BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.policy_bundles_v21 (
  id BIGSERIAL PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(id) ON DELETE CASCADE,

  policy_version_str text NOT NULL,
  policy_version_id  text NULL,

  bundle_artifact_asset_id bigint NOT NULL REFERENCES public.artifact_assets(id) ON DELETE RESTRICT,
  status text NOT NULL CHECK (status IN ('draft','review','published','retired')),
  checksum_sha256 char(64) NOT NULL CHECK (length(checksum_sha256)=64),

  published_at_utc timestamptz NULL,
  published_by_type varchar(16) NULL CHECK (published_by_type IN ('system','user','service')),
  published_by_id   varchar(128) NULL,

  bundle_notes_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  created_at_utc timestamptz NOT NULL DEFAULT now(),
  updated_at_utc timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT policy_bundles_v21_project_nonempty CHECK (btrim(project_id::text) <> ''),
  CONSTRAINT policy_bundles_v21_policy_ver_nonempty CHECK (btrim(policy_version_str) <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_policy_bundles_v21_project_ver
  ON public.policy_bundles_v21(project_id, policy_version_str);

CREATE INDEX IF NOT EXISTS idx_policy_bundles_v21_project_status_time
  ON public.policy_bundles_v21(project_id, status, created_at_utc DESC);

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname='set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_policy_bundles_v21_updated_at') THEN
      CREATE TRIGGER trg_policy_bundles_v21_updated_at
      BEFORE UPDATE ON public.policy_bundles_v21
      FOR EACH ROW
      EXECUTE FUNCTION set_updated_at();
    END IF;
  END IF;
END$$;

COMMIT;