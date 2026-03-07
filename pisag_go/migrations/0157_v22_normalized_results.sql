BEGIN;

CREATE TABLE IF NOT EXISTS normalized_multimodal_results (
    id bigserial PRIMARY KEY,
    project_id text NOT NULL,
    trace_id text NOT NULL,
    run_id text NOT NULL,
    task_id bigint NOT NULL REFERENCES multimodal_tasks(id) ON DELETE CASCADE,
    result_id bigint NOT NULL UNIQUE REFERENCES multimodal_results(id) ON DELETE CASCADE,

    normalized_kind text NOT NULL,
    normalized_status text NOT NULL DEFAULT 'ready',

    summary_text text NOT NULL DEFAULT '',
    confidence_score numeric(5,4),

    reason_code text NOT NULL DEFAULT '',

    review_payload_evidence_asset_id bigint REFERENCES evidence_assets(id) ON DELETE SET NULL,
    downstream_payload_evidence_asset_id bigint REFERENCES evidence_assets(id) ON DELETE SET NULL,

    created_at_utc timestamptz NOT NULL DEFAULT now(),
    updated_at_utc timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_normalized_multimodal_results_project_run
    ON normalized_multimodal_results(project_id, run_id);

CREATE INDEX IF NOT EXISTS idx_normalized_multimodal_results_task
    ON normalized_multimodal_results(task_id);

CREATE INDEX IF NOT EXISTS idx_normalized_multimodal_results_status
    ON normalized_multimodal_results(project_id, normalized_status);

COMMIT;