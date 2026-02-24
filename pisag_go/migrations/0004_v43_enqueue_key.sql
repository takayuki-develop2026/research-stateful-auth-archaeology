-- migrations/0004_v43_enqueue_key.sql (revised)
-- 목적: run_inputs.enqueue_key を導入し、重複排除キーを持たせる
-- 方針: enqueue_key は NOT NULL + DEFAULT でDBが自動生成できる（呼び出し側の事故防止）

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE public.run_inputs
  ADD COLUMN IF NOT EXISTS enqueue_key text;

-- ① DEFAULT を先に付与（INSERTがenqueue_key未指定でも必ず入る）
ALTER TABLE public.run_inputs
  ALTER COLUMN enqueue_key
  SET DEFAULT encode(
    digest(coalesce(method,'GET') || '|' || coalesce(target_url,''), 'sha256'),
    'hex'
  );

-- ② 既存行に埋める（過去のNULLも救済）
UPDATE public.run_inputs
SET enqueue_key = encode(
  digest(coalesce(method,'GET') || '|' || coalesce(target_url,''), 'sha256'),
  'hex'
)
WHERE enqueue_key IS NULL;

-- ③ NOT NULL を確定
ALTER TABLE public.run_inputs
  ALTER COLUMN enqueue_key SET NOT NULL;

-- ④ run_id + enqueue_key で重複を許さない（現状設計のまま）
CREATE UNIQUE INDEX IF NOT EXISTS run_inputs_run_enqueue_uniq
  ON public.run_inputs (run_id, enqueue_key);

COMMIT;