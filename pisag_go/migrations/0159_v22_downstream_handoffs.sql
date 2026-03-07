BEGIN;

CREATE TABLE IF NOT EXISTS multimodal_downstream_handoffs (
    id bigserial PRIMARY KEY,
    project_id text NOT NULL,
    trace_id text NOT NULL,
    run_id text NOT NULL,

    task_id bigint NOT NULL REFERENCES multimodal_tasks(id) ON DELETE CASCADE,
    result_id bigint NOT NULL REFERENCES multimodal_results(id) ON DELETE CASCADE,
    normalized_result_id bigint NOT NULL REFERENCES normalized_multimodal_results(id) ON DELETE CASCADE,

    destination_kind text NOT NULL,
    payload_evidence_asset_id bigint NOT NULL REFERENCES evidence_assets(id) ON DELETE RESTRICT,

    handoff_status text NOT NULL DEFAULT 'pending',
    reason_code text NOT NULL DEFAULT '',

    created_at_utc timestamptz NOT NULL DEFAULT now(),
    updated_at_utc timestamptz NOT NULL DEFAULT now(),
    delivered_at_utc timestamptz
);

CREATE INDEX IF NOT EXISTS idx_multimodal_downstream_handoffs_project_status
    ON multimodal_downstream_handoffs(project_id, handoff_status);

CREATE INDEX IF NOT EXISTS idx_multimodal_downstream_handoffs_run
    ON multimodal_downstream_handoffs(project_id, run_id);

CREATE INDEX IF NOT EXISTS idx_multimodal_downstream_handoffs_task
    ON multimodal_downstream_handoffs(task_id);

COMMIT;