-- migrations/0009_fix_evidence_trace_uuid.sql
-- 목적: 既存DBで run_evidence_assets.trace_id が text のまま残っている問題を矯正
-- 注意: trace_id が uuid文字列で保存されていることが前提。違う値が混ざると失敗する。

BEGIN;

-- 既存値がuuidへキャストできるか事前チェック（失敗しそうならここで止まる）
DO $$
DECLARE
  bad_count int;
BEGIN
  SELECT count(*) INTO bad_count
  FROM public.run_evidence_assets
  WHERE trace_id IS NOT NULL
    AND trace_id !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$';

  IF bad_count > 0 THEN
    RAISE EXCEPTION 'run_evidence_assets.trace_id has % non-uuid values', bad_count;
  END IF;
END$$;

ALTER TABLE public.run_evidence_assets
  ALTER COLUMN trace_id TYPE uuid USING trace_id::uuid;

COMMIT;