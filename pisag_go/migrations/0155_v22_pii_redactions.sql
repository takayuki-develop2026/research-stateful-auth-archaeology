BEGIN;

CREATE TABLE IF NOT EXISTS pii_redactions (
    id BIGSERIAL PRIMARY KEY,
    project_id text NOT NULL,
    trace_id text NOT NULL,
    evidence_id bigint NOT NULL,

    policy_decision_id bigint NOT NULL,
    rule_key text NOT NULL,
    action text NOT NULL,
    applied_by_type text NOT NULL,
    applied_by_id text NULL,

    detail_evidence_asset_id bigint NOT NULL,
    created_at_utc timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_pii_redactions_action
        CHECK (action IN ('mask', 'deny', 'allow')),

    CONSTRAINT chk_pii_redactions_applied_by_type
        CHECK (applied_by_type IN ('system', 'human')),

    CONSTRAINT fk_pii_redactions_evidence
        FOREIGN KEY (evidence_id)
        REFERENCES evidence_assets(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_pii_redactions_detail_evidence
        FOREIGN KEY (detail_evidence_asset_id)
        REFERENCES evidence_assets(id)
        ON DELETE RESTRICT
);

COMMIT;