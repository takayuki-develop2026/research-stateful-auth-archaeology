CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE run_inputs
  ADD COLUMN IF NOT EXISTS enqueue_key text;

-- 既存行に暫定キーを入れる（最初は雑でもOK。後で再計算してもいい）
UPDATE run_inputs
SET enqueue_key = encode(digest(coalesce(run_id::text,'') || '|' || coalesce(method,'GET') || '|' || coalesce(target_url,''), 'sha256'), 'hex')
WHERE enqueue_key IS NULL;

ALTER TABLE run_inputs
  ALTER COLUMN enqueue_key SET NOT NULL;

-- run_id + enqueue_key で重複を許さない
CREATE UNIQUE INDEX IF NOT EXISTS run_inputs_run_enqueue_uniq
  ON run_inputs (run_id, enqueue_key);