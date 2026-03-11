BEGIN;

CREATE TABLE IF NOT EXISTS public.ocr_results (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES public.multimodal_tasks(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL,
    engine_kind TEXT NOT NULL,
    engine_version TEXT NOT NULL,
    language TEXT NULL,
    raw_text TEXT NOT NULL DEFAULT '',
    blocks_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    avg_score NUMERIC(10,4) NULL,
    payload_evidence_asset_id BIGINT NOT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
    confidence_evidence_asset_id BIGINT NULL REFERENCES public.evidence_assets(id) ON DELETE RESTRICT,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ocr_results_task_id
    ON public.ocr_results(task_id);

CREATE INDEX IF NOT EXISTS idx_ocr_results_project_id
    ON public.ocr_results(project_id);

CREATE INDEX IF NOT EXISTS idx_ocr_results_engine_kind
    ON public.ocr_results(engine_kind);

CREATE INDEX IF NOT EXISTS idx_ocr_results_payload_asset_id
    ON public.ocr_results(payload_evidence_asset_id);

CREATE INDEX IF NOT EXISTS idx_ocr_results_confidence_asset_id
    ON public.ocr_results(confidence_evidence_asset_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_ocr_results_task_engine_payload
    ON public.ocr_results(task_id, engine_kind, payload_evidence_asset_id);

COMMIT;