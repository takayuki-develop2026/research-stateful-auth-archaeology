-- migrations/0007_worker_exec_only.sql (policy A: minimal)
-- 목적: runs status update を SECURITY DEFINER function 経由に固定

BEGIN;

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

REVOKE ALL ON FUNCTION public.runs_mark_done(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.runs_mark_failed(uuid,text,text) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION public.runs_mark_done(uuid) TO ak_worker;
GRANT EXECUTE ON FUNCTION public.runs_mark_failed(uuid,text,text) TO ak_worker;

COMMIT;