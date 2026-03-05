-- 0094_v24_api_contract_diffs.sql
-- v24 Phase A: Contract diffs produced by CI (artifact-based)
BEGIN;

CREATE TABLE IF NOT EXISTS public.api_contract_diffs (
  id BIGSERIAL PRIMARY KEY,

  contract_name TEXT NOT NULL,

  from_release_id BIGINT NOT NULL,
  to_release_id BIGINT NOT NULL,

  diff_sha256 TEXT NOT NULL CHECK (public.is_hex_sha256(lower(btrim(diff_sha256)))),

  -- diff report location (artifact_ref; html/json/junit/etc)
  diff_report_artifact_ref TEXT NOT NULL,

  breaking_change_count INT NOT NULL DEFAULT 0 CHECK (breaking_change_count >= 0),

  created_at_utc TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT fk_api_contract_diffs_from_release
    FOREIGN KEY (from_release_id) REFERENCES public.api_contract_releases(id) ON DELETE CASCADE,

  CONSTRAINT fk_api_contract_diffs_to_release
    FOREIGN KEY (to_release_id) REFERENCES public.api_contract_releases(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_api_contract_diffs_name_created
  ON public.api_contract_diffs(contract_name, created_at_utc);

CREATE INDEX IF NOT EXISTS idx_api_contract_diffs_from_to
  ON public.api_contract_diffs(from_release_id, to_release_id);

COMMIT;