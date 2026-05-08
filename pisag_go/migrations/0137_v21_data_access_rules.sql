BEGIN;

CREATE TABLE IF NOT EXISTS public.data_access_rules_v21 (
  id BIGSERIAL PRIMARY KEY,
  project_id varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  rule_key text NOT NULL,
  rule_spec_evidence_asset_id bigint NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,

  enabled boolean NOT NULL DEFAULT true,

  created_at_utc timestamptz NOT NULL DEFAULT now(),
  updated_at_utc timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT dar_v21_rule_nonempty CHECK (btrim(rule_key) <> ''),
  CONSTRAINT dar_v21_project_nonempty CHECK (btrim(project_id::text) <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_data_access_rules_v21_project_rule
  ON public.data_access_rules_v21(project_id, rule_key);

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname='set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_data_access_rules_v21_updated_at') THEN
      CREATE TRIGGER trg_data_access_rules_v21_updated_at
      BEFORE UPDATE ON public.data_access_rules_v21
      FOR EACH ROW
      EXECUTE FUNCTION set_updated_at();
    END IF;
  END IF;
END$$;

COMMIT;