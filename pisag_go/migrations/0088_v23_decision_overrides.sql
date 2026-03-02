-- 0088_v23_decision_overrides.sql
-- v23: Optional structured override record (reason/attachments are assets). Note: override always creates NEW decision_ledger.

BEGIN;

CREATE TABLE IF NOT EXISTS decision_overrides_v23 (
  decision_id BIGINT PRIMARY KEY REFERENCES decision_ledgers_v23(id) ON DELETE CASCADE,

  override_reason_evidence_asset_id BIGINT NOT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT,
  attachments_evidence_asset_id BIGINT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT,

  created_at_utc TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;