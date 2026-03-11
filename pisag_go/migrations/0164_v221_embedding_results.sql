BEGIN;

CREATE TABLE IF NOT EXISTS public.embedding_results (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES public.multimodal_tasks(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL,
    engine_kind TEXT NOT NULL,
    engine_version TEXT NOT NULL,
    embedding_vector_ref TEXT NOT NULL,
    embedding_dim INTEGER NOT NULL,
    top_candidates_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    payload_evidence_asset_id BIGINT NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_embedding_dim_positive CHECK (embedding_dim > 0)
);

CREATE INDEX IF NOT EXISTS idx_embedding_results_task_id
    ON public.embedding_results(task_id);

CREATE INDEX IF NOT EXISTS idx_embedding_results_project_id
    ON public.embedding_results(project_id);

CREATE INDEX IF NOT EXISTS idx_embedding_results_engine_kind
    ON public.embedding_results(engine_kind);

CREATE INDEX IF NOT EXISTS idx_embedding_results_vector_ref
    ON public.embedding_results(embedding_vector_ref);

CREATE INDEX IF NOT EXISTS idx_embedding_results_payload_asset_id
    ON public.embedding_results(payload_evidence_asset_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_embedding_results_task_engine_vector_ref
    ON public.embedding_results(task_id, engine_kind, embedding_vector_ref);

COMMIT;