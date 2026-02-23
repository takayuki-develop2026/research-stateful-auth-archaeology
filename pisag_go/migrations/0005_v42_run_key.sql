ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS run_key text;

-- project_id + run_key で run を再利用できる
CREATE UNIQUE INDEX IF NOT EXISTS runs_project_run_key_uniq
  ON runs (project_id, run_key)
  WHERE run_key IS NOT NULL;