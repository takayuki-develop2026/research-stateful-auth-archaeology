BEGIN;

ALTER TABLE evidence_assets
    ADD COLUMN IF NOT EXISTS run_id text,
    ADD COLUMN IF NOT EXISTS trace_id text,
    ADD COLUMN IF NOT EXISTS kind text,
    ADD COLUMN IF NOT EXISTS parent_evidence_id bigint,
    ADD COLUMN IF NOT EXISTS meta_evidence_asset_id bigint;

ALTER TABLE evidence_assets
    ADD CONSTRAINT fk_evidence_assets_parent_evidence
        FOREIGN KEY (parent_evidence_id)
        REFERENCES evidence_assets(id)
        ON DELETE SET NULL;

ALTER TABLE evidence_assets
    ADD CONSTRAINT fk_evidence_assets_meta_evidence
        FOREIGN KEY (meta_evidence_asset_id)
        REFERENCES evidence_assets(id)
        ON DELETE SET NULL;

COMMIT;