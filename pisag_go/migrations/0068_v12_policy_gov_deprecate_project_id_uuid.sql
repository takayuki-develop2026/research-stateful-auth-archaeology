BEGIN;

-- v12: project_id(uuid) is deprecated in favor of project_key(text)
-- Make old uuid columns nullable to avoid blocking inserts.

ALTER TABLE gov_policy.policy_sets
  ALTER COLUMN project_id DROP NOT NULL;

ALTER TABLE gov_policy.policy_publications
  ALTER COLUMN project_id DROP NOT NULL;

-- (optional) If you want to prevent future writes to uuid columns,
-- you can leave them NULL and use project_key only.

COMMIT;