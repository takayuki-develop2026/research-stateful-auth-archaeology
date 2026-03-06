BEGIN;

CREATE TABLE IF NOT EXISTS multimodal_tasks (
    id BIGSERIAL PRIMARY KEY,
    project_id text NOT NULL,
    trace_id text NOT NULL,
    run_id text NOT NULL,

    task_key text NOT NULL,
    task_type text NOT NULL,
    pipeline_version text NOT NULL,
    policy_version_str text NOT NULL,
    input_hash text NOT NULL,

    status text NOT NULL,
    router_plan_evidence_asset_id bigint NOT NULL,
    options_evidence_asset_id bigint NOT NULL,
    model_run_id bigint NULL,

    started_at_utc timestamptz NULL,
    finished_at_utc timestamptz NULL,
    soft_error_evidence_asset_id bigint NULL,

    created_at_utc timestamptz NOT NULL DEFAULT now(),
    updated_at_utc timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_multimodal_tasks_project_task_key
        UNIQUE (project_id, task_key),

    CONSTRAINT chk_multimodal_tasks_task_type
        CHECK (
            task_type IN (
                'fulltext_extract',
                'ocr',
                'vision',
                'audio_transcribe',
                'audio_classify'
            )
        ),

    CONSTRAINT chk_multimodal_tasks_status
        CHECK (
            status IN (
                'queued',
                'running',
                'succeeded',
                'review_required',
                'skipped_budget',
                'failed_soft',
                'blocked_policy'
            )
        ),

    CONSTRAINT fk_multimodal_tasks_router_plan_evidence
        FOREIGN KEY (router_plan_evidence_asset_id)
        REFERENCES evidence_assets(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_multimodal_tasks_options_evidence
        FOREIGN KEY (options_evidence_asset_id)
        REFERENCES evidence_assets(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_multimodal_tasks_soft_error_evidence
        FOREIGN KEY (soft_error_evidence_asset_id)
        REFERENCES evidence_assets(id)
        ON DELETE SET NULL
);

COMMIT;