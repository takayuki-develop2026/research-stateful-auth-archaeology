BEGIN;

CREATE TABLE IF NOT EXISTS multimodal_result_outputs (
    id BIGSERIAL PRIMARY KEY,
    project_id text NOT NULL,
    result_id bigint NOT NULL,
    evidence_id bigint NOT NULL,
    output_role text NOT NULL,
    seq integer NOT NULL,

    created_at_utc timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_multimodal_result_outputs_result_evidence_role_seq
        UNIQUE (result_id, evidence_id, output_role, seq),

    CONSTRAINT chk_multimodal_result_outputs_seq_non_negative
        CHECK (seq >= 0),

    CONSTRAINT fk_multimodal_result_outputs_result
        FOREIGN KEY (result_id)
        REFERENCES multimodal_results(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_multimodal_result_outputs_evidence
        FOREIGN KEY (evidence_id)
        REFERENCES evidence_assets(id)
        ON DELETE RESTRICT
);

COMMIT;