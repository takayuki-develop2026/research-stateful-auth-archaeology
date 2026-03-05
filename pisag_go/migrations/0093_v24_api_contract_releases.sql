-- 0093_v24_api_contract_releases.sql
-- v24 Phase A: Contract Release Ledger (Single Source of Contract metadata)
-- Asset-id-only: long notes go to evidence_assets via notes_evidence_asset_id
BEGIN;

CREATE TABLE IF NOT EXISTS public.api_contract_releases (
  id BIGSERIAL PRIMARY KEY,

  -- contract identity
  contract_name TEXT NOT NULL, -- e.g. 'atlaskernel-openapi'

  -- semver
  major INT NOT NULL,
  minor INT NOT NULL,
  patch INT NOT NULL,

  contract_format TEXT NOT NULL CHECK (contract_format IN ('yaml','json')),
  contract_sha256 TEXT NOT NULL CHECK (public.is_hex_sha256(lower(btrim(contract_sha256)))),

  -- where the contract file is stored (artifact_ref string, e.g. contract://openapi/v1.yaml or artifact ref)
  contract_artifact_ref TEXT NOT NULL,

  status TEXT NOT NULL CHECK (status IN ('draft','review','published','deprecated','sunset')),

  published_at_utc TIMESTAMPTZ NULL,

  -- asset-id-only notes (optional)
  notes_evidence_asset_id BIGINT NULL,

  created_by_type TEXT NOT NULL CHECK (created_by_type IN ('user','service')),
  created_by_id TEXT NOT NULL,

  created_at_utc TIMESTAMPTZ NOT NULL DEFAULT now(),

  UNIQUE (contract_name, major, minor, patch)
);

CREATE INDEX IF NOT EXISTS idx_api_contract_releases_name_status
  ON public.api_contract_releases(contract_name, status);

CREATE INDEX IF NOT EXISTS idx_api_contract_releases_published_at
  ON public.api_contract_releases(published_at_utc);

-- notes_evidence_asset_id: best-effort FK to evidence_assets (may be NULL)
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema='public' AND table_name='evidence_assets'
  ) THEN
    BEGIN
      ALTER TABLE public.api_contract_releases
        ADD CONSTRAINT fk_api_contract_releases_notes_evidence
        FOREIGN KEY (notes_evidence_asset_id)
        REFERENCES public.evidence_assets(id)
        ON DELETE SET NULL;
    EXCEPTION WHEN duplicate_object THEN
      -- ignore
    END;
  END IF;
END $$;

COMMIT;