BEGIN;

ALTER TABLE public.multimodal_tasks
ADD COLUMN IF NOT EXISTS engine_selection_json jsonb NOT NULL DEFAULT '{}'::jsonb;

COMMIT;