-- migrations/0056_v50_provider_routes.sql
-- v5.0: provider_routes (candidate route set)
-- Depends: projects, providers
BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS public.provider_routes (
  route_id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id      varchar(26) NOT NULL REFERENCES public.projects(project_id) ON DELETE CASCADE,

  provider_id     uuid NOT NULL REFERENCES public.providers(provider_id) ON DELETE RESTRICT,

  status          varchar(16) NOT NULL DEFAULT 'active', -- active|inactive|blocked
  priority        int NOT NULL DEFAULT 100,

  region          varchar(16) NOT NULL,
  currency        varchar(8)  NOT NULL,
  payment_method  varchar(32) NOT NULL,

  constraints     jsonb NOT NULL DEFAULT '{}'::jsonb, -- lightweight (schema validated in CI)
  weights         jsonb NOT NULL DEFAULT '{}'::jsonb, -- lightweight (schema validated in CI)
  why_policy_ref  varchar(32) NOT NULL,               -- references policy_version

  meta            jsonb NOT NULL DEFAULT '{}'::jsonb,

  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='provider_routes_status_ck') THEN
    ALTER TABLE public.provider_routes
      ADD CONSTRAINT provider_routes_status_ck CHECK (lower(status::text) IN ('active','inactive','blocked'));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='provider_routes_region_nonempty') THEN
    ALTER TABLE public.provider_routes
      ADD CONSTRAINT provider_routes_region_nonempty CHECK (btrim(region::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='provider_routes_currency_nonempty') THEN
    ALTER TABLE public.provider_routes
      ADD CONSTRAINT provider_routes_currency_nonempty CHECK (btrim(currency::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='provider_routes_method_nonempty') THEN
    ALTER TABLE public.provider_routes
      ADD CONSTRAINT provider_routes_method_nonempty CHECK (btrim(payment_method::text) <> '');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='provider_routes_policy_ref_nonempty') THEN
    ALTER TABLE public.provider_routes
      ADD CONSTRAINT provider_routes_policy_ref_nonempty CHECK (btrim(why_policy_ref::text) <> '');
  END IF;
END$$;

-- indexes
CREATE INDEX IF NOT EXISTS idx_provider_routes_project_filter
  ON public.provider_routes(project_id, status, region, currency, payment_method);

CREATE INDEX IF NOT EXISTS idx_provider_routes_project_provider
  ON public.provider_routes(project_id, provider_id);

CREATE INDEX IF NOT EXISTS idx_provider_routes_project_priority
  ON public.provider_routes(project_id, priority);

-- updated_at trigger (if exists)
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_proc WHERE proname='set_updated_at') THEN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='trg_provider_routes_updated_at') THEN
      CREATE TRIGGER trg_provider_routes_updated_at
      BEFORE UPDATE ON public.provider_routes
      FOR EACH ROW
      EXECUTE FUNCTION set_updated_at();
    END IF;
  END IF;
END$$;

COMMIT;