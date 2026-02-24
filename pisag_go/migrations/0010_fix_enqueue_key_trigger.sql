-- migrations/0010_fix_enqueue_key_trigger.sql
-- 목적: enqueue_key をDB側で必ず生成する（DEFAULTでは列参照できないため trigger で対応）
-- 方針:
-- - INSERT時、enqueue_key が NULL/空なら method+target_url から sha256 hex を生成
-- - 既存NULL行も埋める

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 1) enqueue_key 生成関数
CREATE OR REPLACE FUNCTION public.run_inputs_set_enqueue_key()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public
AS $$
BEGIN
  IF NEW.enqueue_key IS NULL OR btrim(NEW.enqueue_key) = '' THEN
    NEW.enqueue_key := encode(
      digest(coalesce(NEW.method,'GET') || '|' || coalesce(NEW.target_url,''), 'sha256'),
      'hex'
    );
  END IF;
  RETURN NEW;
END;
$$;

-- 2) trigger が無ければ作る
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_run_inputs_set_enqueue_key') THEN
    CREATE TRIGGER trg_run_inputs_set_enqueue_key
    BEFORE INSERT ON public.run_inputs
    FOR EACH ROW
    EXECUTE FUNCTION public.run_inputs_set_enqueue_key();
  END IF;
END$$;

-- 3) 既存のNULLを埋める（今回の failing row を救済）
UPDATE public.run_inputs
SET enqueue_key = encode(
  digest(coalesce(method,'GET') || '|' || coalesce(target_url,''), 'sha256'),
  'hex'
)
WHERE enqueue_key IS NULL OR btrim(enqueue_key) = '';

COMMIT;