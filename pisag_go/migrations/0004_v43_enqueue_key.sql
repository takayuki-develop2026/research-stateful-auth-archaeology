BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'projects'
      AND column_name = 'id'
  )
  AND NOT EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'projects'
      AND column_name = 'project_id'
  ) THEN
    ALTER TABLE public.projects RENAME COLUMN id TO project_id;
  END IF;
END
$$;

ALTER TABLE public.run_inputs
ADD COLUMN IF NOT EXISTS enqueue_key text;

UPDATE public.run_inputs
SET enqueue_key = encode(
  digest(coalesce(method, 'GET') || '|' || coalesce(target_url, ''), 'sha256'),
  'hex'
)
WHERE enqueue_key IS NULL;

ALTER TABLE public.run_inputs
ALTER COLUMN enqueue_key SET DEFAULT '';

ALTER TABLE public.run_inputs
ALTER COLUMN enqueue_key SET NOT NULL;

DROP INDEX IF EXISTS run_inputs_run_enqueue_uniq;

CREATE UNIQUE INDEX run_inputs_run_enqueue_uniq
ON public.run_inputs (run_id, enqueue_key);

COMMIT;
