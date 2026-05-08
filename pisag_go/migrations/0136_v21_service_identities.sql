BEGIN;

CREATE TABLE IF NOT EXISTS public.service_identities_v21 (
  id BIGSERIAL PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  service_name text NOT NULL,

  audiences_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
  jwks_artifact_asset_id bigint NULL REFERENCES public.artifact_assets(id) ON DELETE RESTRICT,
  jwks_url_evidence_asset_id bigint NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  status text NOT NULL CHECK (status IN ('active','rotating','retired')),
  rotated_at_utc timestamptz NULL,

  created_at_utc timestamptz NOT NULL DEFAULT now(),
  updated_at_utc timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT svc_ids_v21_service_nonempty CHECK (btrim(service_name) <> ''),
  CONSTRAINT svc_ids_v21_project_nonempty CHECK (btrim(project_id::text) <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_service_identities_v21_project_service
  ON public.service_identities_v21(project_id, service_name);

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname='set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_service_identities_v21_updated_at') THEN
      CREATE TRIGGER trg_service_identities_v21_updated_at
      BEFORE UPDATE ON public.service_identities_v21
      FOR EACH ROW
      EXECUTE FUNCTION set_updated_at();
    END IF;
  END IF;
END$$;

COMMIT;