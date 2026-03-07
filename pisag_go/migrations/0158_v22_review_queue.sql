BEGIN;

CREATE TABLE IF NOT EXISTS multimodal_review_queue (
    id bigserial PRIMARY KEY,
    project_id text NOT NULL,
    trace_id text NOT NULL,
    run_id text NOT NULL,

    task_id bigint NOT NULL REFERENCES multimodal_tasks(id) ON DELETE CASCADE,
    result_id bigint NOT NULL REFERENCES multimodal_results(id) ON DELETE CASCADE,
    normalized_result_id bigint NOT NULL REFERENCES normalized_multimodal_results(id) ON DELETE CASCADE,

    queue_status text NOT NULL DEFAULT 'pending',
    priority text NOT NULL DEFAULT 'normal',
    reason_code text NOT NULL DEFAULT '',

    assigned_reviewer_id text NOT NULL DEFAULT '',

    created_at_utc timestamptz NOT NULL DEFAULT now(),
    updated_at_utc timestamptz NOT NULL DEFAULT now(),
    resolved_at_utc timestamptz
);

CREATE INDEX IF NOT EXISTS idx_multimodal_review_queue_project_status
    ON multimodal_review_queue(project_id, queue_status);

CREATE INDEX IF NOT EXISTS idx_multimodal_review_queue_run
    ON multimodal_review_queue(project_id, run_id);

CREATE INDEX IF NOT EXISTS idx_multimodal_review_queue_task
    ON multimodal_review_queue(task_id);

COMMIT;