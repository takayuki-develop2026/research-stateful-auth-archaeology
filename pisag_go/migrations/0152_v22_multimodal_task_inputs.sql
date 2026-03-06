BEGIN;

CREATE TABLE IF NOT EXISTS multimodal_task_inputs (
    id BIGSERIAL PRIMARY KEY,
    project_id text NOT NULL,
    task_id bigint NOT NULL,
    evidence_id bigint NOT NULL,
    input_role text NOT NULL,
    seq integer NOT NULL,

    created_at_utc timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_multimodal_task_inputs_task_evidence_role_seq
        UNIQUE (task_id, evidence_id, input_role, seq),

    CONSTRAINT chk_multimodal_task_inputs_seq_non_negative
        CHECK (seq >= 0),

    CONSTRAINT fk_multimodal_task_inputs_task
        FOREIGN KEY (task_id)
        REFERENCES multimodal_tasks(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_multimodal_task_inputs_evidence
        FOREIGN KEY (evidence_id)
        REFERENCES evidence_assets(id)
        ON DELETE RESTRICT
);

COMMIT;