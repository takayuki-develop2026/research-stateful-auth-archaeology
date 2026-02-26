BEGIN;

-- Allow either evidence_assets payload OR run_evidence_assets payload
ALTER TABLE public.dlq_items_v13
  ALTER COLUMN payload_evidence_asset_id DROP NOT NULL;

COMMIT;

