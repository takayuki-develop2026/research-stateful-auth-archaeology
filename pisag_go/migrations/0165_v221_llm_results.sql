BEGIN;

CREATE TABLE IF NOT EXISTS public.llm_results (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES public.multimodal_tasks(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL,
    engine_kind TEXT NOT NULL,
    engine_version TEXT NOT NULL,
    provider TEXT NOT NULL,
    task_kind TEXT NOT NULL,
    input_hash TEXT NOT NULL,
    output_text TEXT NOT NULL DEFAULT '',
    output_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    rationale_text TEXT NOT NULL DEFAULT '',
    prompt_version TEXT NOT NULL DEFAULT '',
    token_usage_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    cost_estimate NUMERIC(18,6) NULL,
    payload_evidence_asset_id BIGINT NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
    confidence_evidence_asset_id BIGINT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_llm_results_task_id
    ON public.llm_results(task_id);

CREATE INDEX IF NOT EXISTS idx_llm_results_project_id
    ON public.llm_results(project_id);

CREATE INDEX IF NOT EXISTS idx_llm_results_engine_kind
    ON public.llm_results(engine_kind);

CREATE INDEX IF NOT EXISTS idx_llm_results_provider
    ON public.llm_results(provider);

CREATE INDEX IF NOT EXISTS idx_llm_results_task_kind
    ON public.llm_results(task_kind);

CREATE INDEX IF NOT EXISTS idx_llm_results_input_hash
    ON public.llm_results(input_hash);

CREATE INDEX IF NOT EXISTS idx_llm_results_payload_asset_id
    ON public.llm_results(payload_evidence_asset_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_llm_results_task_engine_input_hash
    ON public.llm_results(task_id, engine_kind, task_kind, input_hash);

COMMIT;