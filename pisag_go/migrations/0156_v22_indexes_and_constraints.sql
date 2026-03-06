BEGIN;

CREATE INDEX IF NOT EXISTS idx_evidence_assets_project_kind
    ON evidence_assets(project_id, kind);

CREATE INDEX IF NOT EXISTS idx_evidence_assets_trace_id
    ON evidence_assets(trace_id);

CREATE INDEX IF NOT EXISTS idx_evidence_assets_run_id
    ON evidence_assets(project_id, run_id);

CREATE INDEX IF NOT EXISTS idx_evidence_assets_parent_evidence_id
    ON evidence_assets(parent_evidence_id);

CREATE INDEX IF NOT EXISTS idx_evidence_assets_meta_evidence_asset_id
    ON evidence_assets(meta_evidence_asset_id);

CREATE INDEX IF NOT EXISTS idx_multimodal_tasks_project_run
    ON multimodal_tasks(project_id, run_id);

CREATE INDEX IF NOT EXISTS idx_multimodal_tasks_project_task_type_status
    ON multimodal_tasks(project_id, task_type, status);

CREATE INDEX IF NOT EXISTS idx_multimodal_tasks_trace_id
    ON multimodal_tasks(trace_id);

CREATE INDEX IF NOT EXISTS idx_multimodal_tasks_model_run_id
    ON multimodal_tasks(model_run_id);

CREATE INDEX IF NOT EXISTS idx_multimodal_task_inputs_task_seq
    ON multimodal_task_inputs(task_id, seq);

CREATE INDEX IF NOT EXISTS idx_multimodal_task_inputs_project_task
    ON multimodal_task_inputs(project_id, task_id);

CREATE INDEX IF NOT EXISTS idx_multimodal_results_project_run
    ON multimodal_results(project_id, run_id);

CREATE INDEX IF NOT EXISTS idx_multimodal_results_task_id
    ON multimodal_results(task_id);

CREATE INDEX IF NOT EXISTS idx_multimodal_results_trace_id
    ON multimodal_results(trace_id);

CREATE INDEX IF NOT EXISTS idx_multimodal_result_outputs_result_seq
    ON multimodal_result_outputs(result_id, seq);

CREATE INDEX IF NOT EXISTS idx_pii_redactions_project_created
    ON pii_redactions(project_id, created_at_utc);

CREATE INDEX IF NOT EXISTS idx_pii_redactions_trace_id
    ON pii_redactions(trace_id);

CREATE INDEX IF NOT EXISTS idx_pii_redactions_evidence_id
    ON pii_redactions(evidence_id);

COMMIT;