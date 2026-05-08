-- migrations/0055_v50_providers.sql
-- v5.0: providers (canonical PSP/provider registry)
-- Depends: v18 projects(id varchar(26))
BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.providers (
  provider_id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id    varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  provider_key  varchar(64) NOT NULL, -- stripe|adyen|...
  status        varchar(16) NOT NULL DEFAULT 'active', -- active|degraded|blocked

  capabilities  jsonb NOT NULL DEFAULT '{}'::jsonb, -- lightweight
  meta          jsonb NOT NULL DEFAULT '{}'::jsonb, -- lightweight

  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

-- constraints
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='providers_provider_key_nonempty') THEN
    ALTER TABLE public.providers
      ADD CONSTRAINT providers_provider_key_nonempty CHECK (btrim(provider_key::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='providers_status_ck') THEN
    ALTER TABLE public.providers
      ADD CONSTRAINT providers_status_ck CHECK (lower(status::text) IN ('active','degraded','blocked'));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ux_providers_project_key') THEN
    ALTER TABLE public.providers
      ADD CONSTRAINT ux_providers_project_key UNIQUE (project_id, provider_key);
  END IF;
END$$;

-- indexes
CREATE INDEX IF NOT EXISTS idx_providers_project_status
  ON public.providers(project_id, status);

-- updated_at trigger (if exists)
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname='set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_providers_updated_at') THEN
      CREATE TRIGGER trg_providers_updated_at
      BEFORE UPDATE ON public.providers
      FOR EACH ROW
      EXECUTE FUNCTION set_updated_at();
    END IF;
  END IF;
END$$;

COMMIT;