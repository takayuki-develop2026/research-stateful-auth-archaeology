BEGIN;

CREATE TABLE IF NOT EXISTS public.preprocess_results (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES public.multimodal_tasks(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL,
    source_evidence_asset_id BIGINT NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
    engine_kind TEXT NOT NULL,
    engine_version TEXT NOT NULL,
    operations_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    output_evidence_asset_id BIGINT NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
    quality_score NUMERIC(10,4) NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_preprocess_results_task_id
    ON public.preprocess_results(task_id);

CREATE INDEX IF NOT EXISTS idx_preprocess_results_project_id
    ON public.preprocess_results(project_id);

CREATE INDEX IF NOT EXISTS idx_preprocess_results_source_asset_id
    ON public.preprocess_results(source_evidence_asset_id);

CREATE INDEX IF NOT EXISTS idx_preprocess_results_output_asset_id
    ON public.preprocess_results(output_evidence_asset_id);

CREATE INDEX IF NOT EXISTS idx_preprocess_results_engine_kind
    ON public.preprocess_results(engine_kind);

CREATE UNIQUE INDEX IF NOT EXISTS uq_preprocess_results_task_engine_output
    ON public.preprocess_results(task_id, engine_kind, output_evidence_asset_id);

COMMIT;