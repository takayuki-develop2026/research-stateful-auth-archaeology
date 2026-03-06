BEGIN;

CREATE TABLE IF NOT EXISTS multimodal_results (
    id BIGSERIAL PRIMARY KEY,
    project_id text NOT NULL,
    trace_id text NOT NULL,
    run_id text NOT NULL,
    task_id bigint NOT NULL,

    result_key text NOT NULL,
    result_type text NOT NULL,
    output_hash text NOT NULL,

    payload_evidence_asset_id bigint NOT NULL,
    confidence_evidence_asset_id bigint NULL,

    created_at_utc timestamptz NOT NULL DEFAULT now(),
    updated_at_utc timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_multimodal_results_project_result_key
        UNIQUE (project_id, result_key),

    CONSTRAINT fk_multimodal_results_task
        FOREIGN KEY (task_id)
        REFERENCES multimodal_tasks(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_multimodal_results_payload_evidence
        FOREIGN KEY (payload_evidence_asset_id)
        REFERENCES evidence_assets(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_multimodal_results_confidence_evidence
        FOREIGN KEY (confidence_evidence_asset_id)
        REFERENCES evidence_assets(id)
        ON DELETE SET NULL
);

COMMIT;