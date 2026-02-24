-- migrations/0011_run_input_status_fns.sql
-- 목적:
-- - ak_worker は run_inputs を直接 UPDATE できない（write-only + execute-only）
-- - そのため run_inputs の状態遷移を SECURITY DEFINER 関数に集約する
--
-- 提供する関数:
-- - public.run_inputs_mark_done(input_id, worker_id)
-- - public.run_inputs_mark_retry(input_id, worker_id, code, msg)
-- - public.run_inputs_touch_claim(input_id, worker_id)
-- - public.run_inputs_set_next_attempt_at(input_id, next_at)
--
-- 方針:
-- - claim_status='claimed' かつ claimed_by が一致する場合のみ更新
-- - retry は指数バックオフ（attempt_count * 10 秒、最大300秒）
-- - terminal は done 扱い（last_error_* を保持）
-- - 失敗時に例外で落とさず、更新0件は false を返す（運用で扱いやすい）
--
-- 注意:
-- - run_inputs.claim_status に 'done' を使う（現状 text なので許容）
-- - claimed_at/claimed_by の取り扱いは “doneで解放 / retryで解放” に統一
-- - worker が他workerのclaimを触れないことをDBで保証

BEGIN;

-- 0) ユーティリティ: terminal判定（SQL側で必要なら）
--    今回は retry 関数がそのまま retry を作るだけにし、
--    terminal は呼び出し側が code を見て mark_done にするかで分岐しても良い。
--    ただし要望に合わせて「retry内でterminal化」も実装する。

CREATE OR REPLACE FUNCTION public.run_inputs_is_terminal_code(p_code text)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
AS $$
  SELECT CASE
    WHEN p_code IS NULL OR btrim(p_code) = '' THEN false
    WHEN p_code IN ('fetch_denied','http_400','http_401','http_403','http_404','http_410') THEN true
    ELSE false
  END
$$;

REVOKE ALL ON FUNCTION public.run_inputs_is_terminal_code(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.run_inputs_is_terminal_code(text) TO ak_worker;

-- 1) done: claimを解放して完了にする（last_errorはクリア）
CREATE OR REPLACE FUNCTION public.run_inputs_mark_done(
  p_input_id bigint,
  p_worker_id text
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_updated int;
BEGIN
  IF p_input_id IS NULL OR p_input_id <= 0 THEN
    RAISE EXCEPTION 'input_id is required';
  END IF;
  IF p_worker_id IS NULL OR btrim(p_worker_id) = '' THEN
    RAISE EXCEPTION 'worker_id is required';
  END IF;

  UPDATE public.run_inputs
  SET claim_status='done',
      last_error_code=NULL,
      last_error_message=NULL,
      claimed_at=NULL,
      claimed_by=NULL
  WHERE id=p_input_id
    AND claim_status='claimed'
    AND claimed_by=p_worker_id;

  GET DIAGNOSTICS v_updated = ROW_COUNT;
  RETURN (v_updated = 1);
END;
$$;

REVOKE ALL ON FUNCTION public.run_inputs_mark_done(bigint,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.run_inputs_mark_done(bigint,text) TO ak_worker;

-- 2) retry: 失敗情報を記録し、pendingに戻して次回実行時刻を進める
--    terminal code の場合は done に落とす（last_errorは保持）
CREATE OR REPLACE FUNCTION public.run_inputs_mark_retry(
  p_input_id bigint,
  p_worker_id text,
  p_code text,
  p_msg  text
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_updated int;
  v_is_terminal boolean;
BEGIN
  IF p_input_id IS NULL OR p_input_id <= 0 THEN
    RAISE EXCEPTION 'input_id is required';
  END IF;
  IF p_worker_id IS NULL OR btrim(p_worker_id) = '' THEN
    RAISE EXCEPTION 'worker_id is required';
  END IF;
  IF p_code IS NULL OR btrim(p_code) = '' THEN
    RAISE EXCEPTION 'error_code is required';
  END IF;

  v_is_terminal := public.run_inputs_is_terminal_code(p_code);

  IF v_is_terminal THEN
    UPDATE public.run_inputs
    SET claim_status='done',
        last_error_code=p_code,
        last_error_message=p_msg,
        claimed_at=NULL,
        claimed_by=NULL
    WHERE id=p_input_id
      AND claim_status='claimed'
      AND claimed_by=p_worker_id;

    GET DIAGNOSTICS v_updated = ROW_COUNT;
    RETURN (v_updated = 1);
  END IF;

  UPDATE public.run_inputs
  SET claim_status='pending',
      claimed_at=NULL,
      claimed_by=NULL,
      last_error_code=p_code,
      last_error_message=p_msg,
      next_attempt_at = now() + make_interval(secs => LEAST(attempt_count * 10, 300))
  WHERE id=p_input_id
    AND claim_status='claimed'
    AND claimed_by=p_worker_id;

  GET DIAGNOSTICS v_updated = ROW_COUNT;
  RETURN (v_updated = 1);
END;
$$;

REVOKE ALL ON FUNCTION public.run_inputs_mark_retry(bigint,text,text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.run_inputs_mark_retry(bigint,text,text,text) TO ak_worker;

-- 3) touch: heartbeat（claimed_at を更新）
CREATE OR REPLACE FUNCTION public.run_inputs_touch_claim(
  p_input_id bigint,
  p_worker_id text
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_updated int;
BEGIN
  IF p_input_id IS NULL OR p_input_id <= 0 THEN
    RAISE EXCEPTION 'input_id is required';
  END IF;
  IF p_worker_id IS NULL OR btrim(p_worker_id) = '' THEN
    RAISE EXCEPTION 'worker_id is required';
  END IF;

  UPDATE public.run_inputs
  SET claimed_at = now()
  WHERE id=p_input_id
    AND claim_status='claimed'
    AND claimed_by=p_worker_id;

  GET DIAGNOSTICS v_updated = ROW_COUNT;
  RETURN (v_updated = 1);
END;
$$;

REVOKE ALL ON FUNCTION public.run_inputs_touch_claim(bigint,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.run_inputs_touch_claim(bigint,text) TO ak_worker;

-- 4) set_next_attempt_at: 次回試行時刻を手動で指定（運用/管理用途）
--    安全のため "claimed 以外" にしか適用しない（claimed中に弄ると競合する）
CREATE OR REPLACE FUNCTION public.run_inputs_set_next_attempt_at(
  p_input_id bigint,
  p_next_attempt_at timestamptz
)
RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_updated int;
BEGIN
  IF p_input_id IS NULL OR p_input_id <= 0 THEN
    RAISE EXCEPTION 'input_id is required';
  END IF;
  IF p_next_attempt_at IS NULL THEN
    RAISE EXCEPTION 'next_attempt_at is required';
  END IF;

  UPDATE public.run_inputs
  SET next_attempt_at = p_next_attempt_at
  WHERE id=p_input_id
    AND claim_status <> 'claimed';

  GET DIAGNOSTICS v_updated = ROW_COUNT;
  RETURN (v_updated >= 1);
END;
$$;

REVOKE ALL ON FUNCTION public.run_inputs_set_next_attempt_at(bigint,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.run_inputs_set_next_attempt_at(bigint,timestamptz) TO ak_worker;

COMMIT;