BEGIN;

ALTER TABLE gov_policy.policy_sets
  ADD COLUMN IF NOT EXISTS project_key text;

ALTER TABLE gov_policy.policy_publications
  ADD COLUMN IF NOT EXISTS project_key text;

-- best-effort backfill from uuid -> text (if old columns exist)
DO $$
BEGIN
  BEGIN
    UPDATE gov_policy.policy_sets
       SET project_key = project_id::text
     WHERE project_key IS NULL;
  EXCEPTION WHEN others THEN
  END;

  BEGIN
    UPDATE gov_policy.policy_publications
       SET project_key = project_id::text
     WHERE project_key IS NULL;
  EXCEPTION WHEN others THEN
  END;
END $$;

ALTER TABLE gov_policy.policy_sets
  ALTER COLUMN project_key SET NOT NULL;

ALTER TABLE gov_policy.policy_publications
  ALTER COLUMN project_key SET NOT NULL;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM pg_indexes
     WHERE schemaname='gov_policy' AND indexname='gov_policy_sets_uq_project_name'
  ) THEN
    EXECUTE 'DROP INDEX gov_policy.gov_policy_sets_uq_project_name';
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS gov_policy_sets_uq_projectkey_name
  ON gov_policy.policy_sets(project_key, name);

CREATE INDEX IF NOT EXISTS gov_policy_sets_idx_projectkey_status
  ON gov_policy.policy_sets(project_key, status);

COMMIT;