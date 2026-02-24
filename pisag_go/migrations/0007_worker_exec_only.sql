-- migrations/0007_worker_exec_only.sql
-- 목적: ak_worker を「関数EXECUTEのみで runs を更新」へ寄せる（テーブルUPDATE禁止）
-- 前提: owner=ak で実行する

BEGIN;

-- 1) 念のため search_path 固定（SECURITY DEFINER の定石）
--    ※関数内で public を明示しているが、さらに固定して安全側へ。

CREATE OR REPLACE FUNCTION public.runs_mark_done(p_run_id uuid)
RETURNS void
LANGUAGE sql
SECURITY DEFINER
SET search_path = public
AS $$
  UPDATE public.runs
  SET status='done', finished_at=now()
  WHERE run_id = p_run_id;
$$;

CREATE OR REPLACE FUNCTION public.runs_mark_failed(p_run_id uuid, p_code text, p_msg text)
RETURNS void
LANGUAGE sql
SECURITY DEFINER
SET search_path = public
AS $$
  UPDATE public.runs
  SET status='failed',
      finished_at=now(),
      error_code=p_code,
      error_message=p_msg
  WHERE run_id = p_run_id;
$$;

-- 2) ak_worker には「関数実行」だけ付与
GRANT EXECUTE ON FUNCTION public.runs_mark_done(uuid) TO ak_worker;
GRANT EXECUTE ON FUNCTION public.runs_mark_failed(uuid,text,text) TO ak_worker;

-- 3) runs テーブル直 UPDATE を剥奪（ここが本体）
REVOKE UPDATE ON public.runs FROM ak_worker;

-- 4) 念のため SELECT も剥奪（事故防止）
REVOKE SELECT ON public.runs FROM ak_worker;

COMMIT;