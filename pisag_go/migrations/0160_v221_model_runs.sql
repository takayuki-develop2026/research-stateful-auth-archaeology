BEGIN;

CREATE TABLE IF NOT EXISTS public.model_runs (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES public.multimodal_tasks(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL,
    capability TEXT NOT NULL,
    engine_kind TEXT NOT NULL,
    engine_version TEXT NOT NULL,
    provider TEXT NOT NULL,
    task_kind TEXT NULL,
    status TEXT NOT NULL,
    input_hash TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ NULL,
    latency_ms BIGINT NULL,
    token_usage_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    cost_estimate NUMERIC(18,6) NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_model_runs_task_id
    ON public.model_runs(task_id);

CREATE INDEX IF NOT EXISTS idx_model_runs_project_id
    ON public.model_runs(project_id);

CREATE INDEX IF NOT EXISTS idx_model_runs_capability
    ON public.model_runs(capability);

CREATE INDEX IF NOT EXISTS idx_model_runs_engine_kind
    ON public.model_runs(engine_kind);

CREATE INDEX IF NOT EXISTS idx_model_runs_status
    ON public.model_runs(status);

CREATE INDEX IF NOT EXISTS idx_model_runs_started_at
    ON public.model_runs(started_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS uq_model_runs_task_engine_started
    ON public.model_runs(task_id, capability, engine_kind, started_at);

COMMIT;